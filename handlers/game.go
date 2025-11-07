package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/namishh/holmes/services"
	"github.com/namishh/holmes/views/pages"
	"github.com/namishh/holmes/views/pages/hunt"
	"golang.org/x/crypto/bcrypt"
)

func (ah *AuthHandler) HomeHandler(c echo.Context) error {
	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}
	sess, _ := session.Get(auth_sessions_key, c)
	// isError = false
	homeView := pages.Home(fromProtected)
	c.Set("ISERROR", false)
	if auth, ok := sess.Values[auth_key].(bool); !ok || !auth {
		return renderView(c, pages.HomeIndex(
			"Home",
			"",
			fromProtected,
			c.Get("ISERROR").(bool),
			homeView,
		))
	}

	return renderView(c, pages.HomeIndex(
		"Home",
		sess.Values[user_name_key].(string),
		fromProtected,
		c.Get("ISERROR").(bool),
		homeView,
	))
}

func (ah *AuthHandler) Hunt(c echo.Context) error {
	teamID := c.Get(user_id_key).(int)
	questions, err := ah.UserServices.GetAllQuestionsWithStatus(teamID)
	if err != nil {
		return err
	}
	hasCompleted, err := ah.UserServices.HasCompletedAllQuestions(teamID)
	if err != nil {
		return err
	}
	
	// Get quota information
	quotaSlot, err := ah.UserServices.GetQuotaSlot(teamID)
	if err != nil {
		return err
	}
	
	// Get actual completed questions count
	actualCount, err := ah.UserServices.GetActualCompletedQuestionsCount(teamID)
	if err != nil {
		return err
	}
	
	// Update quota slot display with actual count
	if quotaSlot != nil {
		quotaSlot.QuestionsSolvedInSlot = actualCount
	}
	
	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}
	quizview := hunt.Hunt(fromProtected, questions, hasCompleted, quotaSlot)
	c.Set("ISERROR", false)
	return renderView(c, hunt.HuntIndex(
		"Hunt",
		c.Get(user_name_key).(string),
		fromProtected,
		c.Get("ISERROR").(bool),
		quizview,
	))
}

func (ah *AuthHandler) UnlockHint(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return err
	}

	hastaken, err := ah.UserServices.HasTeamUnlockedHint(c.Get(user_id_key).(int), id)
	if err != nil {
		return err
	}

	hint, worth, err := ah.UserServices.GetHintById(id)

	user, _ := ah.UserServices.CheckUsername(c.Get(user_name_key).(string))

	if !hastaken {
		if user.Points < worth {
			quizview := hunt.OutOfPoints()
			c.Set("ISERROR", true)
			fromProtected, _ := c.Get("FROMPROTECTED").(bool)
			return renderView(c, hunt.OutOfPointsIndex(
				"Hint",
				c.Get(user_name_key).(string),
				fromProtected,
				c.Get("ISERROR").(bool),
				quizview,
			))
		}
		err := ah.UserServices.UnlockHintForTeam(c.Get(user_id_key).(int), id, worth)
		if err != nil {
			return err
		}
	}

	if err != nil {
		return err
	}

	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}
	quizview := hunt.Hint(fromProtected, hastaken, hint)
	c.Set("ISERROR", false)
	return renderView(c, hunt.HintIndex(
		"Hint",
		c.Get(user_name_key).(string),
		fromProtected,
		c.Get("ISERROR").(bool),
		quizview,
	))
}

func (ah *AuthHandler) Question(c echo.Context) error {
	errs := make(map[string]string)
	lvl, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return err
	}
	
	teamID := c.Get(user_id_key).(int)
	
	question, err := ah.UserServices.GetQuestionById(lvl)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error fetching question")
	}
	media, err := ah.UserServices.GetMediaByQuestionId(lvl)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error fetching media: %s", err))
	}

	hasCompleted, err := ah.UserServices.IsQuestionSolvedByTeam(teamID, lvl)
	if err != nil {
		return err
	}

	// Check quota - can the team solve more questions in this time slot?
	canSolve, quotaSlot, err := ah.UserServices.CanSolveQuestion(teamID)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error checking quota: %s", err))
	}
	
	if !canSolve && !hasCompleted {
		timeRemaining, _ := ah.UserServices.GetTimeUntilQuotaReset(teamID)
		hours := int(timeRemaining.Hours())
		minutes := int(timeRemaining.Minutes()) % 60
		return c.String(http.StatusForbidden, fmt.Sprintf("Question quota exhausted! You've solved %d/%d questions in this slot. New slot starts in %dh %dm", 
			quotaSlot.QuestionsSolvedInSlot, 10, hours, minutes))
	}

	// Check if question has been solved by ANYONE
	solvedByAnyone, err := ah.UserServices.IsQuestionSolvedByAnyone(lvl)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error checking if question is solved: %s", err))
	}

	// If question is already solved by someone, it should not be lockable
	if solvedByAnyone && !hasCompleted {
		return c.String(http.StatusForbidden, "This question has already been solved by another team")
	}

	// Check if question is locked by another user
	isLocked, lockInfo, err := ah.UserServices.IsQuestionLocked(lvl)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error checking lock status: %s", err))
	}
	
	// If locked by another user, deny access
	if isLocked && lockInfo.LockedByTeamID != teamID {
		return c.String(http.StatusForbidden, fmt.Sprintf("Question is currently being solved by %s", lockInfo.LockedByName))
	}

	hints, err := ah.UserServices.GetHintsByQuestionID(lvl)
	if err != nil {
		return err
	}

	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}

	if c.Request().Method == "POST" {
		sess, _ := session.Get(auth_sessions_key, c)
		if auth := sess.Values[user_type]; auth == "admin" {
			return c.String(http.StatusForbidden, "Admin cannot solve questions")
		}

		if hasCompleted {
			return c.String(http.StatusForbidden, "Question already solved")
		}

		// Check if question attempts are exhausted
		exhausted, err := ah.UserServices.IsQuestionExhausted(teamID, lvl)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Error checking attempts: %s", err))
		}
		if exhausted {
			return c.String(http.StatusForbidden, "Maximum attempts (5) reached for this question")
		}

		answer := c.FormValue("answer")
		if bcrypt.CompareHashAndPassword([]byte(question.Answer), []byte(answer)) == nil {
			// Correct Answer
			// Stop the timer
			err = ah.UserServices.StopQuestionTimer(teamID, lvl)
			if err != nil {
				log.Printf("Warning: Error stopping timer: %s", err)
			}
			
			err = ah.UserServices.MarkQuestionAsCompleted(teamID, lvl)
			if err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Error Validating: %s", err))
			}
			err = ah.UserServices.AddPointsToTeam(teamID, question.Points)
			if err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Error adding Points: %s", err))
			}
			err = ah.UserServices.UpdateTeamLastAnsweredQuestion(teamID)
			if err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Error updating time: %s", err))
			}
			
			// Increment quota count
			err = ah.UserServices.IncrementQuotaCount(teamID)
			if err != nil {
				log.Printf("Warning: Error incrementing quota count: %s", err)
			}
			
			// Unlock the question after successful submission
			err = ah.UserServices.UnlockQuestion(lvl)
			if err != nil {
				log.Printf("Warning: Error unlocking question: %s", err)
			} else {
				// Broadcast unlock and solve events
				ah.Broadcaster.Broadcast(services.EventQuestionUnlocked, map[string]interface{}{
					"question_id": lvl,
				})
				ah.Broadcaster.Broadcast(services.EventQuestionSolved, map[string]interface{}{
					"question_id": lvl,
					"team_id":     teamID,
					"team_name":   c.Get(user_name_key).(string),
					"points":      question.Points,
				})
				ah.Broadcaster.Broadcast(services.EventLeaderboardUpdate, map[string]interface{}{
					"message": "Leaderboard updated",
				})
			}
			
			return c.Redirect(http.StatusFound, "/hunt")
		}

		// Wrong Answer - Apply negative marking
		penalty, attemptsLeft, err := ah.UserServices.RecordWrongAttempt(teamID, lvl, question.Points)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Error recording attempt: %s", err))
		}

		// Deduct penalty points from team's score
		if penalty > 0 {
			err = ah.UserServices.DeductPenaltyPoints(teamID, penalty)
			if err != nil {
				log.Printf("Warning: Error deducting penalty: %s", err)
			}
		}

		// Set error messages with penalty information
		if penalty == 0 {
			errs["answer"] = fmt.Sprintf("Incorrect Answer! This is your warning. You have %d attempts left.", attemptsLeft)
		} else if attemptsLeft > 0 {
			errs["answer"] = fmt.Sprintf("Incorrect Answer! -%d points penalty. You have %d attempts left.", penalty, attemptsLeft)
		} else {
			errs["answer"] = fmt.Sprintf("Incorrect Answer! -%d points penalty. No more attempts left!", penalty)
			// Unlock the question as attempts are exhausted
			err = ah.UserServices.UnlockQuestion(lvl)
			if err != nil {
				log.Printf("Warning: Error unlocking question: %s", err)
			} else {
				// Broadcast unlock event
				ah.Broadcaster.Broadcast(services.EventQuestionUnlocked, map[string]interface{}{
					"question_id": lvl,
					"reason":      "max_attempts_reached",
				})
			}
		}

		// Get updated attempt info to pass to template
		attemptInfo, _ := ah.UserServices.GetQuestionAttempts(teamID, lvl)
		
		quizview := hunt.Question(fromProtected, question, hasCompleted, media, errs, hints, attemptInfo)
		c.Set("ISERROR", false)
		return renderView(c, hunt.QuestionIndex(
			"Solve",
			c.Get(user_name_key).(string),
			fromProtected,
			c.Get("ISERROR").(bool),
			quizview,
		))
	}

	// GET request - Check attempts, lock the question and start timer
	// Check if question attempts are exhausted
	exhausted, err := ah.UserServices.IsQuestionExhausted(teamID, lvl)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error checking attempts: %s", err))
	}
	if exhausted {
		return c.String(http.StatusForbidden, "Maximum attempts (5) reached for this question")
	}

	if !hasCompleted && !isLocked {
		// Lock the question for this user (atomic operation)
		err = ah.UserServices.LockQuestion(lvl, teamID)
		if err != nil {
			log.Printf("Warning: Error locking question: %s", err)
		} else {
			// Broadcast lock event to all connected clients
			ah.Broadcaster.Broadcast(services.EventQuestionLocked, map[string]interface{}{
				"question_id": lvl,
				"team_id":     teamID,
				"team_name":   c.Get(user_name_key).(string),
			})
		}
		
		// Start the timer
		err = ah.UserServices.StartQuestionTimer(teamID, lvl)
		if err != nil {
			log.Printf("Warning: Error starting timer: %s", err)
		}
	}

	// Get attempt info to display to user
	attemptInfo, _ := ah.UserServices.GetQuestionAttempts(teamID, lvl)

	quizview := hunt.Question(fromProtected, question, hasCompleted, media, errs, hints, attemptInfo)
	c.Set("ISERROR", false)
	return renderView(c, hunt.QuestionIndex(
		"Solve",
		c.Get(user_name_key).(string),
		fromProtected,
		c.Get("ISERROR").(bool),
		quizview,
	))
}

func (ah *AuthHandler) Leaderboard(c echo.Context) error {

	fromProtected, ok := c.Get("FROMPROTECTED").(bool)
	if !ok {
		return errors.New("invalid type for key 'FROMPROTECTED'")
	}

	users, err := ah.UserServices.GetLeaderbaord()

	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Error fetching Leaderboard: %s", err))
	}

	user := services.User{}

	if c.Get(user_name_key).(string) == "admin" {
		user = services.User{ID: 0, Username: "Admin", Points: 0}
	} else {
		user, err = ah.UserServices.CheckUsername(c.Get(user_name_key).(string))
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Error fetching you: %s", err))
		}
	}

	quizview := hunt.Leaderboard(fromProtected, users, user)
	c.Set("ISERROR", false)
	return renderView(c, hunt.LeaderboardIndex(
		"Leaderboard",
		c.Get(user_name_key).(string),
		fromProtected,
		c.Get("ISERROR").(bool),
		quizview,
	))
}
