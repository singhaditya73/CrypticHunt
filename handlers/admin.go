package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/namishh/holmes/services"
	"github.com/namishh/holmes/views/pages/auth"
	"github.com/namishh/holmes/views/pages/panel"
	"golang.org/x/crypto/bcrypt"
)

// AdminLoginAttempt tracks failed login attempts for rate limiting
type AdminLoginAttempt struct {
	Count      int
	LastAttempt time.Time
	BlockedUntil time.Time
}

// AdminRateLimiter manages admin login attempts
type AdminRateLimiter struct {
	attempts map[string]*AdminLoginAttempt
	mu       sync.RWMutex
	maxAttempts int
	blockDuration time.Duration
	windowDuration time.Duration
}

var adminRateLimiter = &AdminRateLimiter{
	attempts:       make(map[string]*AdminLoginAttempt),
	maxAttempts:    5,                    // 5 attempts allowed
	blockDuration:  15 * time.Minute,     // Block for 15 minutes after max attempts
	windowDuration: 10 * time.Minute,     // Reset counter after 10 minutes of no attempts
}

// CheckAndRecordAttempt checks if IP is blocked and records the attempt
func (arl *AdminRateLimiter) CheckAndRecordAttempt(ip string, success bool) (bool, time.Duration) {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	now := time.Now()
	
	// Get or create attempt record
	attempt, exists := arl.attempts[ip]
	if !exists {
		attempt = &AdminLoginAttempt{
			Count:       0,
			LastAttempt: now,
		}
		arl.attempts[ip] = attempt
	}

	// Check if currently blocked
	if now.Before(attempt.BlockedUntil) {
		remainingTime := attempt.BlockedUntil.Sub(now)
		return false, remainingTime // Blocked
	}

	// Reset counter if window has passed
	if now.Sub(attempt.LastAttempt) > arl.windowDuration {
		attempt.Count = 0
		attempt.BlockedUntil = time.Time{}
	}

	// Update last attempt time
	attempt.LastAttempt = now

	if success {
		// Successful login - reset counter
		attempt.Count = 0
		attempt.BlockedUntil = time.Time{}
		return true, 0
	}

	// Failed attempt - increment counter
	attempt.Count++

	// Block if max attempts reached
	if attempt.Count >= arl.maxAttempts {
		attempt.BlockedUntil = now.Add(arl.blockDuration)
		log.Printf("Admin login blocked for IP %s until %v (too many failed attempts)", ip, attempt.BlockedUntil)
		return false, arl.blockDuration
	}

	return true, 0 // Not blocked yet
}

// GetRemainingAttempts returns how many attempts are left before blocking
func (arl *AdminRateLimiter) GetRemainingAttempts(ip string) int {
	arl.mu.RLock()
	defer arl.mu.RUnlock()

	attempt, exists := arl.attempts[ip]
	if !exists {
		return arl.maxAttempts
	}

	now := time.Now()
	
	// If blocked, return 0
	if now.Before(attempt.BlockedUntil) {
		return 0
	}

	// If window passed, return max
	if now.Sub(attempt.LastAttempt) > arl.windowDuration {
		return arl.maxAttempts
	}

	remaining := arl.maxAttempts - attempt.Count
	if remaining < 0 {
		return 0
	}
	return remaining
}

// CleanupOldEntries removes old entries (run periodically)
func (arl *AdminRateLimiter) CleanupOldEntries() {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	now := time.Now()
	for ip, attempt := range arl.attempts {
		// Remove entries that haven't been accessed in 1 hour and aren't blocked
		if now.Sub(attempt.LastAttempt) > time.Hour && now.After(attempt.BlockedUntil) {
			delete(arl.attempts, ip)
		}
	}
}

// CleanupAdminRateLimiter is exported for periodic cleanup from main
func CleanupAdminRateLimiter() {
	adminRateLimiter.CleanupOldEntries()
	log.Println("Admin rate limiter cleanup completed")
}

func csrfMiddleware() echo.MiddlewareFunc {
	return middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup:    "form:_csrf",
		CookieName:     "_csrf",
		CookiePath:     "/",
		CookieHTTPOnly: true,
		CookieSecure:   true, // Set to true if using HTTPS
	})
}

func (ah *AuthHandler) adminMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, _ := session.Get(auth_sessions_key, c)
		if user, ok := sess.Values[user_type].(string); !ok || user != "admin" {
			c.Set("FROMPROTECTED", false)
			c.Set("ISADMIN", false)
			// Redirect to sudo login instead of showing error
			return c.Redirect(http.StatusSeeOther, "/sudo")
		}

		c.Set("ISADMIN", true)

		if auth, ok := sess.Values[auth_key].(bool); !ok || !auth {
			c.Set("FROMPROTECTED", false)
			// Redirect to sudo login instead of showing error
			return c.Redirect(http.StatusSeeOther, "/sudo")
		}

		if userId, ok := sess.Values[user_id_key].(int); ok && userId != 0 {
			c.Set(user_id_key, userId) // set the user_id in the context
		}

		if username, ok := sess.Values[user_name_key].(string); ok && len(username) != 0 {
			c.Set(user_name_key, username) // set the username in the context
		}

		if tzone, ok := sess.Values[tzone_key].(string); ok && len(tzone) != 0 {
			c.Set(tzone_key, tzone) // set the client's time zone in the context
		}

		// fromProtected = true
		c.Set("FROMPROTECTED", true)

		return next(c)
	}
}

func (ah *AuthHandler) AdminHandler(c echo.Context) error {
	csrfToken := c.Get("csrf").(string)
	sess, _ := session.Get(auth_sessions_key, c)
	if user, _ := sess.Values[user_type].(string); user == "admin" {
		return c.Redirect(http.StatusSeeOther, "/su")
	}

	errs := make(map[string]string)
	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}

	if c.Request().Method == "POST" {
		// Get client IP for rate limiting
		clientIP := c.RealIP()
		
		// First check if already blocked from previous attempts
		adminRateLimiter.mu.RLock()
		attempt, exists := adminRateLimiter.attempts[clientIP]
		var isBlocked bool
		var blockedFor time.Duration
		if exists {
			now := time.Now()
			isBlocked = now.Before(attempt.BlockedUntil)
			if isBlocked {
				blockedFor = attempt.BlockedUntil.Sub(now)
			}
		}
		adminRateLimiter.mu.RUnlock()
		
		if isBlocked {
			c.Set("ISERROR", true)
			minutes := int(blockedFor.Minutes())
			seconds := int(blockedFor.Seconds()) % 60
			if minutes > 0 {
				errs["pass"] = fmt.Sprintf("Too many failed attempts. Please try again in %d minutes %d seconds", minutes, seconds)
			} else {
				errs["pass"] = fmt.Sprintf("Too many failed attempts. Please try again in %d seconds", seconds)
			}
			
			log.Printf("Blocked admin login attempt from IP: %s", clientIP)

			adminLoginView := auth.AdminLogin(csrfToken, errs)
			c.Set("ISERROR", false)
			return renderView(c, auth.AdminLoginIndex(
				"Admin Panel",
				"admin",
				fromProtected,
				c.Get("ISERROR").(bool),
				adminLoginView,
			))
		}

		// Check password FIRST before recording attempt
		if c.FormValue("password") != os.Getenv("ADMIN_PASS") {
			// Wrong password - NOW record the failed attempt
			adminRateLimiter.CheckAndRecordAttempt(clientIP, false)
			
			c.Set("ISERROR", true)
			
			// Get remaining attempts
			remaining := adminRateLimiter.GetRemainingAttempts(clientIP)
			if remaining > 0 {
				errs["pass"] = fmt.Sprintf("Incorrect Password. %d attempt(s) remaining", remaining)
			} else {
				errs["pass"] = "Incorrect Password. Account temporarily locked"
			}
			
			log.Printf("Failed admin login attempt from IP: %s (remaining: %d)", clientIP, remaining)

			adminLoginView := auth.AdminLogin(csrfToken, errs)
			c.Set("ISERROR", false)
			return renderView(c, auth.AdminLoginIndex(
				"Admin Panel",
				"admin",
				fromProtected,
				c.Get("ISERROR").(bool),
				adminLoginView,
			))
		} else {
			// Successful login - reset rate limiter
			adminRateLimiter.CheckAndRecordAttempt(clientIP, true)
			
			log.Printf("Successful admin login from IP: %s", clientIP)
			
			tzone := ""
			if len(c.Request().Header["X-Timezone"]) != 0 {
				tzone = c.Request().Header["X-Timezone"][0]
			}

			sess, _ := session.Get(auth_sessions_key, c)
			sess.Options = &sessions.Options{
				Path:     "/",
				MaxAge:   60 * 60 * 24 * 7, // 1 week
				HttpOnly: true,
				Secure:   true, // Only send over HTTPS
				SameSite: http.SameSiteStrictMode, // CSRF protection
			}

			// Set user as authenticated, their username,
			// their ID and the client's time zone

			sess.Values = map[interface{}]interface{}{
				auth_key:      true,
				user_type:     "admin",
				user_id_key:   9999999,
				user_name_key: "admin",
				tzone_key:     tzone,
			}
			sess.Save(c.Request(), c.Response())

			return c.Redirect(http.StatusSeeOther, "/su")
		}

	}

	//sess, _ := session.Get(auth_sessions_key, c)
	// isError = false
	adminLoginView := auth.AdminLogin(csrfToken, errs)
	c.Set("ISERROR", false)
	return renderView(c, auth.AdminLoginIndex(
		"Admin Panel",
		"admin",
		fromProtected,
		c.Get("ISERROR").(bool),
		adminLoginView,
	))
}

func (ah *AuthHandler) AdminPageHandler(c echo.Context) error {
	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}

	users := make([]services.User, 0)
	questions := make([]services.Question, 0)

	users, err := ah.UserServices.GetAllUsers()
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error fetching users")
	}

	questions, err = ah.UserServices.GetAllQuestions()
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error fetching questions")
	}

	adminLoginView := panel.PanelHome(fromProtected, users, questions)
	c.Set("ISERROR", false)
	return renderView(c, panel.PanelIndex(
		"Admin Panel",
		"admin",
		fromProtected,
		c.Get("ISERROR").(bool),
		adminLoginView,
	))
}

func (ah *AuthHandler) AdminQuestionHandler(c echo.Context) error {
	errs := make(map[string]string)
	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}

	if c.Request().Method == "POST" {
		// Create a map to store form values for re-rendering
		values := make(map[string]string)
		
		title := c.FormValue("title")
		values["title"] = title

		if len(title) == 0 {
			c.Set("ISERROR", true)
			errs["title"] = "Title cannot be empty"
		}

		question := c.FormValue("question")
		values["question"] = question
		if len(question) == 0 {
			c.Set("ISERROR", true)
			errs["question"] = "Question cannot be empty"
		}

		answer := c.FormValue("answer")
		values["answer"] = answer
		if len(answer) == 0 {
			c.Set("ISERROR", true)
			errs["answer"] = "Answer cannot be empty"
		}

		points := c.FormValue("points")
		values["points"] = points
		i, err := strconv.Atoi(points)
		if err != nil || i == 0 {
			c.Set("ISERROR", true)
			errs["points"] = "Points cannot be empty"
		}

		if len(errs) > 0 {
			questionView := panel.PanelQuestion(fromProtected, errs, values)
			c.Set("ISERROR", false)
			return renderView(c, panel.PanelQuestionIndex(
				"Admin Panel",
				"admin",
				fromProtected,
				c.Get("ISERROR").(bool),
				questionView,
			))
		}

		// create the question
		form, err := c.MultipartForm()
		if err != nil {
			c.Set("ISERROR", true)
			errs["form"] = fmt.Sprintf("Failed to parse form: %v", err)
			adminLoginView := panel.PanelQuestion(fromProtected, errs, values)
			return renderView(c, panel.PanelQuestionIndex(
				"Admin Panel",
				"admin",
				fromProtected,
				c.Get("ISERROR").(bool),
				adminLoginView,
			))
		}
		images, err := ah.UserServices.MakeArray("images", form, "IMG")
		if err != nil {
			c.Set("ISERROR", true)
			errs["images"] = fmt.Sprintf("%v", err)
			adminLoginView := panel.PanelQuestion(fromProtected, errs, values)
			return renderView(c, panel.PanelQuestionIndex(
				"Admin Panel",
				"admin",
				fromProtected,
				c.Get("ISERROR").(bool),
				adminLoginView,
			))
		}
		videos, err := ah.UserServices.MakeArray("videos", form, "VID")
		if err != nil {
			c.Set("ISERROR", true)
			errs["videos"] = fmt.Sprintf("%v", err)
			adminLoginView := panel.PanelQuestion(fromProtected, errs, values)
			return renderView(c, panel.PanelQuestionIndex(
				"Admin Panel",
				"admin",
				fromProtected,
				c.Get("ISERROR").(bool),
				adminLoginView,
			))
		}
		audios, err := ah.UserServices.MakeArray("audios", form, "AUD")
		if err != nil {
			c.Set("ISERROR", true)
			errs["audios"] = fmt.Sprintf("%v", err)
			adminLoginView := panel.PanelQuestion(fromProtected, errs, values)
			return renderView(c, panel.PanelQuestionIndex(
				"Admin Panel",
				"admin",
				fromProtected,
				c.Get("ISERROR").(bool),
				adminLoginView,
			))
		}
		log.Println(images, videos, audios)
		err = ah.UserServices.CreateQuestion(services.Question{Question: question, Title: title, Points: i, Answer: answer}, images, videos, audios)
		if err != nil {
			c.Set("ISERROR", true)
			errs["question"] = fmt.Sprintf("Failed to create question: %v", err)
			adminLoginView := panel.PanelQuestion(fromProtected, errs, values)
			return renderView(c, panel.PanelQuestionIndex(
				"Admin Panel",
				"admin",
				fromProtected,
				c.Get("ISERROR").(bool),
				adminLoginView,
			))
		}
		return c.Redirect(http.StatusSeeOther, "/su")
	}

	adminLoginView := panel.PanelQuestion(fromProtected, errs, map[string]string{})
	c.Set("ISERROR", false)
	return renderView(c, panel.PanelQuestionIndex(
		"Admin Panel",
		"admin",
		fromProtected,
		c.Get("ISERROR").(bool),
		adminLoginView,
	))
}

func (ah *AuthHandler) AdminDeleteTeam(c echo.Context) error {
	teamID := c.Param("id")
	ti, err := strconv.Atoi(teamID)
	if err != nil {
		return echo.NewHTTPError(
			echo.ErrNotFound.Code,
			fmt.Sprintf(
				"something went wrong: %s",
				err,
			))

	}

	ah.UserServices.DeleteTeam(ti)

	return c.Redirect(http.StatusSeeOther, "/su")
}

func (ah *AuthHandler) AdminDeleteQuestion(c echo.Context) error {
	qid := c.Param("id")
	ti, err := strconv.Atoi(qid)
	if err != nil {
		return echo.NewHTTPError(
			echo.ErrNotFound.Code,
			fmt.Sprintf(
				"something went wrong: %s",
				err,
			))

	}

	ah.UserServices.DeleteQuestion(ti)

	return c.Redirect(http.StatusSeeOther, "/su")
}

func (ah *AuthHandler) AdminDeleteHint(c echo.Context) error {
	qid := c.Param("id")
	ti, err := strconv.Atoi(qid)
	if err != nil {
		return echo.NewHTTPError(
			echo.ErrNotFound.Code,
			fmt.Sprintf(
				"something went wrong: %s",
				err,
			))

	}

	ah.UserServices.DeleteHint(ti)

	return c.Redirect(http.StatusSeeOther, "/su/hints")
}
func (ah *AuthHandler) AdminHintsHandler(c echo.Context) error {
	hints, err := ah.UserServices.GetHints()
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error fetching hints")
	}
	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}

	adminLoginView := panel.PanelHints(fromProtected, hints)
	c.Set("ISERROR", false)
	return renderView(c, panel.PanelHintsIndex(
		"Admin Panel",
		"admin",
		fromProtected,
		c.Get("ISERROR").(bool),
		adminLoginView,
	))
}

func (ah *AuthHandler) AdminHintNewHandler(c echo.Context) error {
	errs := make(map[string]string)
	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}

	if c.Request().Method == "POST" {
		title := c.FormValue("title")
		level := c.FormValue("level")
		worth := c.FormValue("worth")

		if len(title) == 0 {
			c.Set("ISERROR", true)
			errs["title"] = "You can not scam people with an empty hint."
		}

		l, err := strconv.Atoi(level)
		if err != nil {
			c.Set("ISERROR", true)
			errs["level"] = "Invalid level"
		}

		_, err = ah.UserServices.GetQuestionById(l)
		if err != nil {
			c.Set("ISERROR", true)
			errs["level"] = "Invalid level"
		}

		w, err := strconv.Atoi(worth)
		if err != nil {
			c.Set("ISERROR", true)
			errs["worth"] = "Invalid worth"
		}

		if len(errs) > 0 {
			adminLoginView := panel.PanelNewHint(fromProtected, errs)
			c.Set("ISERROR", false)
			return renderView(c, panel.PanelNewHintIndex(
				"Admin Panel",
				"admin",
				fromProtected,
				c.Get("ISERROR").(bool),
				adminLoginView,
			))
		}

		err = ah.UserServices.CreateHint(services.Hint{Hint: title, ParentQuestionID: l, Worth: w})
		if err != nil {
			c.Set("ISERROR", true)
			errs["title"] = "Error creating hint"
		}

		return c.Redirect(http.StatusSeeOther, "/su/hints")
	}

	adminLoginView := panel.PanelNewHint(fromProtected, errs)
	c.Set("ISERROR", false)
	return renderView(c, panel.PanelNewHintIndex(
		"Admin Panel",
		"admin",
		fromProtected,
		c.Get("ISERROR").(bool),
		adminLoginView,
	))
}

func (ah *AuthHandler) AdminEditQuestionHandler(c echo.Context) error {
	qid := c.Param("id")
	errs := make(map[string]string)
	inputs := make(map[string]string)
	media := make(map[string][]string)
	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}
	t, err := strconv.Atoi(qid)
	if err != nil {
		return echo.NewHTTPError(
			echo.ErrNotFound.Code,
			fmt.Sprintf(
				"something went wrong: %s",
				err,
			))
	}

	question, err := ah.UserServices.GetQuestionById(t)

	if err != nil {
		return echo.NewHTTPError(
			echo.ErrNotFound.Code,
			fmt.Sprintf(
				"something went wrong: %s",
				err,
			))
	}

	inputs["title"] = question.Title
	inputs["question"] = question.Question
	inputs["points"] = strconv.Itoa(question.Points)

	media["images"], err = ah.UserServices.GetMedia("SELECT path FROM images where parent_question_id = ?", t)
	media["videos"], err = ah.UserServices.GetMedia("SELECT path FROM videos where parent_question_id = ?", t)
	media["audios"], err = ah.UserServices.GetMedia("SELECT path FROM audios where parent_question_id = ?", t)

	media["limages"], err = ah.UserServices.GetMedia("SELECT id FROM images where parent_question_id = ?", t)
	media["lvideos"], err = ah.UserServices.GetMedia("SELECT id FROM videos where parent_question_id = ?", t)
	media["laudios"], err = ah.UserServices.GetMedia("SELECT id FROM audios where parent_question_id = ?", t)

	if c.Request().Method == "POST" {

		form, err := c.MultipartForm()
		if err != nil {
			return err
		}
		images, err := ah.UserServices.MakeArray("images", form, "IMG")
		if err != nil {
			return err
		}
		videos, err := ah.UserServices.MakeArray("videos", form, "VID")
		if err != nil {
			return err
		}
		audios, err := ah.UserServices.MakeArray("audios", form, "AUD")
		if err != nil {
			return err
		}
		log.Println(images, videos, audios)
		err = ah.UserServices.CreateMedia(t, images, videos, audios)

		title := c.FormValue("title")
		qn := c.FormValue("question")
		points := c.FormValue("points")
		answer := c.FormValue("answer")

		if answer == "" {
			answer = question.Answer
		} else {
			by, err := bcrypt.GenerateFromPassword([]byte(answer), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			answer = string(by)
		}

		if len(title) == 0 {
			c.Set("ISERROR", true)
			errs["title"] = "Empty title."
		}

		p, err := strconv.Atoi(points)
		if err != nil || p == 0 {
			c.Set("ISERROR", true)
			errs["points"] = "Invalid Points."
		}

		if len(errs) > 0 {
			view := panel.PanelEditQuestion(fromProtected, errs, inputs, media)

			c.Set("ISERROR", false)

			return renderView(c, panel.PanelEditQuestionIndex(
				"Edit",
				"",
				fromProtected,
				c.Get("ISERROR").(bool),
				view,
			))
		}

		err = ah.UserServices.UpdateQuestion(t, title, qn, p, answer)
		return c.Redirect(http.StatusSeeOther, "/su")
	}

	view := panel.PanelEditQuestion(fromProtected, errs, inputs, media)

	c.Set("ISERROR", false)

	return renderView(c, panel.PanelEditQuestionIndex(
		"Edit",
		"",
		fromProtected,
		c.Get("ISERROR").(bool),
		view,
	))
}

func (ah *AuthHandler) AdminDeleteImage(c echo.Context) error {
	qid := c.Param("name")
	n, err := strconv.Atoi(qid)
	if err != nil {
		return echo.NewHTTPError(
			echo.ErrNotFound.Code,
			fmt.Sprintf(
				"something went wrong: %s",
				err,
			))
	}
	ah.UserServices.DeleteMedia(n, "images")
	return c.Redirect(http.StatusSeeOther, "/su")
}

func (ah *AuthHandler) AdminDeleteAudio(c echo.Context) error {
	qid := c.Param("name")
	n, err := strconv.Atoi(qid)
	if err != nil {
		return echo.NewHTTPError(
			echo.ErrNotFound.Code,
			fmt.Sprintf(
				"something went wrong: %s",
				err,
			))
	}
	ah.UserServices.DeleteMedia(n, "audios")
	return c.Redirect(http.StatusSeeOther, "/su")
}

func (ah *AuthHandler) AdminDeleteVideo(c echo.Context) error {
	qid := c.Param("name")
	n, err := strconv.Atoi(qid)
	if err != nil {
		return echo.NewHTTPError(
			echo.ErrNotFound.Code,
			fmt.Sprintf(
				"something went wrong: %s",
				err,
			))
	}
	ah.UserServices.DeleteMedia(n, "videos")
	return c.Redirect(http.StatusSeeOther, "/su")
}

// AdminSolvedQuestionsHandler shows all solved questions with option to unlock them
func (ah *AuthHandler) AdminSolvedQuestionsHandler(c echo.Context) error {
	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}

	solvedQuestions, err := ah.UserServices.GetAllSolvedQuestions()
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error fetching solved questions: %s", err))
	}

	view := panel.SolvedQuestions(fromProtected, solvedQuestions)
	c.Set("ISERROR", false)

	return renderView(c, panel.SolvedQuestionsIndex(
		"Solved Questions",
		c.Get(user_name_key).(string),
		fromProtected,
		c.Get("ISERROR").(bool),
		view,
	))
}

// AdminUnlockQuestionHandler unlocks a solved question for a specific team
func (ah *AuthHandler) AdminUnlockQuestionHandler(c echo.Context) error {
	qid := c.Param("qid")
	tid := c.Param("tid")
	
	questionID, err := strconv.Atoi(qid)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid question ID")
	}
	
	teamID, err := strconv.Atoi(tid)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid team ID")
	}

	err = ah.UserServices.UnlockSolvedQuestion(questionID, teamID)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error unlocking question: %s", err))
	}

	return c.Redirect(http.StatusSeeOther, "/su/solved-questions")
}

// AdminUnlockAllQuestionHandler unlocks a question for all teams
func (ah *AuthHandler) AdminUnlockAllQuestionHandler(c echo.Context) error {
	qid := c.Param("qid")
	
	questionID, err := strconv.Atoi(qid)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid question ID")
	}

	err = ah.UserServices.UnlockAllSolvedQuestions(questionID)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error unlocking question: %s", err))
	}

	return c.Redirect(http.StatusSeeOther, "/su/solved-questions")
}

