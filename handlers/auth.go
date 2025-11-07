package handlers

import (
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"mime/multipart"

	"github.com/a-h/templ"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/namishh/holmes/database"
	"github.com/namishh/holmes/services"
	"github.com/namishh/holmes/views/pages/auth"
	"golang.org/x/crypto/bcrypt"
)

const auth_key string = "auth_key"
const auth_sessions_key string = "auth_session_key"
const user_id_key string = "user_id_key"
const user_name_key string = "user_name_key"
const tzone_key string = "tzone_key"
const user_type string = "user_type"
const theme string = "gray"
const accent string = "blue"

type AuthService interface {
	CreateUser(u services.User) error
	CheckEmail(email string) (services.User, error)
	CheckUsername(usr string) (services.User, error)

	GetAllUsers() ([]services.User, error)
	DeleteTeam(id int) error

	GetAllQuestions() ([]services.Question, error)
	DeleteQuestion(id int) error
	MakeArray(label string, form *multipart.Form, short string) (list []string, err error)
	CreateQuestion(q services.Question, images []string, video []string, audio []string) error
	CreateMedia(ID int, images []string, videos []string, audios []string) error
	GetQuestionById(id int) (services.Question, error)
	UpdateQuestion(id int, title string, question string, points int, answer string) error
	GetAllQuestionsWithStatus(userID int) ([]services.QuestionWithStatus, error)
	HasCompletedAllQuestions(userID int) (bool, error)
	IsQuestionSolvedByTeam(teamID, questionID int) (bool, error)
	GetMediaByQuestionId(id int) (map[string][]string, error)
	MarkQuestionAsCompleted(userID, questionID int) error
	AddPointsToTeam(teamID int, points int) error
	UpdateTeamLastAnsweredQuestion(teamID int) error

	GetHints() ([]services.Hint, error)
	CreateHint(h services.Hint) error
	DeleteHint(id int) error
	GetHintsByQuestionID(questionID int) ([]services.Hint, error)
	GetHintById(id int) (string, int, error)
	HasTeamUnlockedHint(teamID int, hintID int) (bool, error)
	UnlockHintForTeam(teamID int, hintID int, worth int) error
	GetLeaderbaord() ([]services.LeaderBoardUser, error)

	// Question locking methods
	LockQuestion(questionID int, teamID int) error
	UnlockQuestion(questionID int) error
	IsQuestionLocked(questionID int) (bool, *services.QuestionLock, error)
	IsQuestionSolvedByAnyone(questionID int) (bool, error)
	GetAllLockedQuestions() ([]services.QuestionLock, error)

	// Timer methods
	StartQuestionTimer(teamID int, questionID int) error
	StopQuestionTimer(teamID int, questionID int) error
	GetTotalSolveTime(teamID int) (int, error)
	GetQuestionSolveTime(teamID int, questionID int) (int, error)

	// Attempt and penalty methods
	GetQuestionAttempts(teamID int, questionID int) (*services.QuestionAttempt, error)
	RecordWrongAttempt(teamID int, questionID int, questionPoints int) (int, int, error)
	IsQuestionExhausted(teamID int, questionID int) (bool, error)
	GetTotalPenalty(teamID int) (int, error)
	DeductPenaltyPoints(teamID int, penalty int) error

	// Quota management methods
	GetQuotaSlot(teamID int) (*services.QuotaSlot, error)
	CreateQuotaSlot(teamID int) (*services.QuotaSlot, error)
	ResetQuotaSlot(teamID int) (*services.QuotaSlot, error)
	IncrementQuotaCount(teamID int) error
	CanSolveQuestion(teamID int) (bool, *services.QuotaSlot, error)
	GetTimeUntilQuotaReset(teamID int) (time.Duration, error)
	GetActualCompletedQuestionsCount(teamID int) (int, error)

	// Admin methods
	AdminUnlockQuestion(questionID int) error
	GetSolvedQuestions() ([]services.QuestionWithSolvers, error)
	GetAllSolvedQuestions() ([]services.SolvedQuestionInfo, error)
	UnlockSolvedQuestion(questionID int, teamID int) error
	UnlockAllSolvedQuestions(questionID int) error

	GetMedia(query string, args ...interface{}) ([]string, error)
	GetIdByPath(path string, table string) (int, error)
	DeleteMedia(id int, table string) error

	// Health check methods
	PingDB() error
	GetDBStats() database.DBStats
}

type AuthHandler struct {
	UserServices AuthService
	Broadcaster  *services.Broadcaster
}

func NewAuthHandler(us AuthService, broadcaster *services.Broadcaster) *AuthHandler {
	return &AuthHandler{
		UserServices: us,
		Broadcaster:  broadcaster,
	}
}

func renderView(c echo.Context, cmp templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTML)

	return cmp.Render(c.Request().Context(), c.Response().Writer)
}

func (ah *AuthHandler) flagsMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, _ := session.Get(auth_sessions_key, c)
		if auth, ok := sess.Values[auth_key].(bool); !ok || !auth {
			c.Set("FROMPROTECTED", false)
		} else {
			c.Set("FROMPROTECTED", true)
		}

		if auth := sess.Values[user_type]; auth == "admin" {
			c.Set("ISADMIN", true)
		} else {
			c.Set("ISADMIN", false)
		}

		return next(c)
	}
}

func (ah *AuthHandler) authMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, _ := session.Get(auth_sessions_key, c)
		if auth, ok := sess.Values[auth_key].(bool); !ok || !auth {
			c.Set("FROMPROTECTED", false)
			// Redirect to login instead of showing error
			return c.Redirect(http.StatusSeeOther, "/login")
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

func valid(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func (ah *AuthHandler) LoginHandler(c echo.Context) error {
	errs := make(map[string]string)
	fromProtected, ok := c.Get("FROMPROTECTED").(bool)

	if fromProtected {
		return c.Redirect(http.StatusSeeOther, "/")
	}

	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}

	if c.Request().Method == "POST" {
		tzone := ""
		if len(c.Request().Header["X-Timezone"]) != 0 {
			tzone = c.Request().Header["X-Timezone"][0]
		}

		user, err := ah.UserServices.CheckEmail(c.FormValue("email"))

		log.Print(user)

		if err != nil {
			if strings.Contains(err.Error(), "no rows in result set") {
				c.Set("ISERROR", true)
				errs["dne"] = "User with this email does not exist."
				view := auth.Login(fromProtected, errs)

				return renderView(c, auth.LoginIndex(
					"Login",
					"",
					fromProtected,
					c.Get("ISERROR").(bool),
					view,
				))
			}
		}

		err = bcrypt.CompareHashAndPassword(
			[]byte(user.Password),
			[]byte(c.FormValue("password")),
		)
		if err != nil {
			c.Set("ISERROR", true)
			errs["pass"] = "Incorrect Password"
			view := auth.Login(fromProtected, errs)

			return renderView(c, auth.LoginIndex(
				"Login",
				"",
				fromProtected,
				c.Get("ISERROR").(bool),
				view,
			))
		}

		// Log in the user
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
			user_type:     "ordinary",
			user_id_key:   user.ID,
			user_name_key: user.Username,
			tzone_key:     tzone,
		}
		sess.Save(c.Request(), c.Response())

		return c.Redirect(http.StatusSeeOther, "/hunt")

	}
	// isError = false
	view := auth.Login(fromProtected, errs)
	c.Set("ISERROR", false)

	return renderView(c, auth.LoginIndex(
		"Login",
		"",
		fromProtected,
		c.Get("ISERROR").(bool),
		view,
	))
}

func (ah *AuthHandler) RegisterHandler(c echo.Context) error {

	errs := make(map[string]string)
	fromProtected, ok := c.Get("FROMPROTECTED").(bool)

	if fromProtected {
		return c.Redirect(http.StatusSeeOther, "/")
	}

	if c.Request().Method == "POST" {
		email := c.FormValue("email")
		password := c.FormValue("password")
		username := strings.TrimSpace(c.FormValue("username"))

		if !valid(email) {
			errs["email"] = "Invalid email address"
			c.Set("ISERROR", true)
		}

		_, err := ah.UserServices.CheckEmail(email)
		if err == nil || username == "admin" {
			errs["username"] = "Nuh uh, nice try being the admin"
			c.Set("ISERROR", true)
		}

		// password valid: minimum 8 characters
		if len(password) < 8 {
			errs["password"] = "Password must be at least 8 characters"
			c.Set("ISERROR", true)
		}

		// username valid: minimum 4 letters
		if len(username) < 4 {
			errs["username"] = "Username must be at least 4 characters"
			c.Set("ISERROR", true)
		}
		
		// username valid: only alphanumeric and underscore
		for _, char := range username {
			if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_') {
				errs["username"] = "Username can only contain letters, numbers, and underscores"
				c.Set("ISERROR", true)
				break
			}
		}

		_, err = ah.UserServices.CheckUsername(username)
		log.Print(err)
		if err == nil {
			errs["username"] = "Account with this username already exists"
			c.Set("ISERROR", true)
		}

		if errs["username"] != "" || errs["email"] != "" || errs["password"] != "" {
			view := auth.Register(fromProtected, errs)

			c.Set("ISERROR", false)

			return renderView(c, auth.RegisterIndex(
				"Register",
				"",
				fromProtected,
				c.Get("ISERROR").(bool),
				view,
			))
		}

		user := services.User{
			Email:    email,
			Username: username,
			Password: password,
		}

		ah.UserServices.CreateUser(user)

		return c.Redirect(http.StatusSeeOther, "/login")
	}

	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}

	view := auth.Register(fromProtected, errs)

	c.Set("ISERROR", false)

	return renderView(c, auth.RegisterIndex(
		"Register",
		"",
		fromProtected,
		c.Get("ISERROR").(bool),
		view,
	))
}

func (ah *AuthHandler) LogoutHandler(c echo.Context) error {
	sess, _ := session.Get(auth_sessions_key, c)
	fromProtected, _ := c.Get("FROMPROTECTED").(bool)

	if !fromProtected {
		return c.Redirect(http.StatusSeeOther, "/")
	}
	// Revoke users authentication
	sess.Values = map[interface{}]interface{}{
		auth_key:      false,
		user_id_key:   "",
		user_type:     "none",
		user_name_key: "",
		tzone_key:     "",
	}
	sess.Save(c.Request(), c.Response())

	// fromProtected = false
	c.Set("FROMPROTECTED", false)

	return c.Redirect(http.StatusSeeOther, "/login")
}
