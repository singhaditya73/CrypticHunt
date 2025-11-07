package services

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

type QuestionLock struct {
	QuestionID     int       `json:"question_id"`
	LockedByTeamID int       `json:"locked_by_team_id"`
	LockedByName   string    `json:"locked_by_name"`
	LockedAt       time.Time `json:"locked_at"`
}

type QuestionTimer struct {
	TeamID           int       `json:"team_id"`
	QuestionID       int       `json:"question_id"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at"`
	TimeTakenSeconds int       `json:"time_taken_seconds"`
}

// LockQuestion locks a question for a specific team using atomic operation
// Returns error if question is already locked by another team
func (us *UserService) LockQuestion(questionID int, teamID int) error {
	// First try to lock atomically - only succeeds if not already locked
	query := `INSERT INTO question_locks (question_id, locked_by_team_id, locked_at) 
			  SELECT ?, ?, ?
			  WHERE NOT EXISTS (
				  SELECT 1 FROM question_locks WHERE question_id = ?
			  )`
	
	result, err := us.UserStore.DB.Exec(query, questionID, teamID, time.Now(), questionID)
	if err != nil {
		log.Printf("Error locking question %d for team %d: %v", questionID, teamID, err)
		return err
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rowsAffected == 0 {
		// Question was already locked by someone else
		return fmt.Errorf("question %d is already locked by another team", questionID)
	}
	
	log.Printf("Successfully locked question %d for team %d", questionID, teamID)
	return nil
}

// TryLockQuestion attempts to lock a question, returns true if successful
func (us *UserService) TryLockQuestion(questionID int, teamID int) (bool, error) {
	err := us.LockQuestion(questionID, teamID)
	if err != nil {
		if err.Error() == fmt.Sprintf("question %d is already locked by another team", questionID) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// UnlockQuestion unlocks a question (when submission happens)
func (us *UserService) UnlockQuestion(questionID int) error {
	query := `DELETE FROM question_locks WHERE question_id = ?`
	
	_, err := us.UserStore.DB.Exec(query, questionID)
	if err != nil {
		log.Printf("Error unlocking question %d: %v", questionID, err)
		return err
	}
	
	log.Printf("Successfully unlocked question %d", questionID)
	return nil
}

// IsQuestionLocked checks if a question is locked
// Automatically unlocks questions that have been locked for more than 10 seconds
func (us *UserService) IsQuestionLocked(questionID int) (bool, *QuestionLock, error) {
	// First, clean up stale locks (older than 10 seconds)
	cleanupQuery := `DELETE FROM question_locks 
					 WHERE question_id = ? 
					 AND locked_at < datetime('now', '-10 seconds')`
	_, err := us.UserStore.DB.Exec(cleanupQuery, questionID)
	if err != nil {
		log.Printf("Error cleaning up stale lock for question %d: %v", questionID, err)
	}
	
	query := `SELECT ql.question_id, ql.locked_by_team_id, t.name, ql.locked_at 
			  FROM question_locks ql
			  JOIN teams t ON ql.locked_by_team_id = t.id
			  WHERE ql.question_id = ?`
	
	var lock QuestionLock
	err = us.UserStore.DB.QueryRow(query, questionID).Scan(
		&lock.QuestionID, 
		&lock.LockedByTeamID, 
		&lock.LockedByName,
		&lock.LockedAt,
	)
	
	if err == sql.ErrNoRows {
		return false, nil, nil
	}
	
	if err != nil {
		log.Printf("Error checking if question %d is locked: %v", questionID, err)
		return false, nil, err
	}
	
	return true, &lock, nil
}

// GetAllLockedQuestions returns all currently locked questions
// Automatically cleans up stale locks (older than 10 seconds)
func (us *UserService) GetAllLockedQuestions() ([]QuestionLock, error) {
	// First, clean up all stale locks
	cleanupQuery := `DELETE FROM question_locks WHERE locked_at < datetime('now', '-10 seconds')`
	result, err := us.UserStore.DB.Exec(cleanupQuery)
	if err != nil {
		log.Printf("Error cleaning up stale locks: %v", err)
	} else {
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			log.Printf("Cleaned up %d stale question locks", rowsAffected)
		}
	}
	
	query := `SELECT ql.question_id, ql.locked_by_team_id, t.name, ql.locked_at 
			  FROM question_locks ql
			  JOIN teams t ON ql.locked_by_team_id = t.id`
	
	rows, err := us.UserStore.DB.Query(query)
	if err != nil {
		log.Printf("Error getting all locked questions: %v", err)
		return nil, err
	}
	defer rows.Close()
	
	var locks []QuestionLock
	for rows.Next() {
		var lock QuestionLock
		err := rows.Scan(&lock.QuestionID, &lock.LockedByTeamID, &lock.LockedByName, &lock.LockedAt)
		if err != nil {
			log.Printf("Error scanning locked question: %v", err)
			return nil, err
		}
		locks = append(locks, lock)
	}
	
	return locks, nil
}

// StartQuestionTimer starts the timer when a user opens a question
func (us *UserService) StartQuestionTimer(teamID int, questionID int) error {
	// Check if timer already exists
	var exists int
	checkQuery := `SELECT COUNT(*) FROM question_timers WHERE team_id = ? AND question_id = ?`
	err := us.UserStore.DB.QueryRow(checkQuery, teamID, questionID).Scan(&exists)
	if err != nil {
		log.Printf("Error checking timer existence: %v", err)
		return err
	}
	
	// Only start timer if it doesn't exist yet
	if exists == 0 {
		query := `INSERT INTO question_timers (team_id, question_id, started_at) 
				  VALUES (?, ?, ?)`
		
		_, err := us.UserStore.DB.Exec(query, teamID, questionID, time.Now())
		if err != nil {
			log.Printf("Error starting timer for team %d, question %d: %v", teamID, questionID, err)
			return err
		}
		
		log.Printf("Successfully started timer for team %d, question %d", teamID, questionID)
	}
	
	return nil
}

// StopQuestionTimer stops the timer and records the solve time
func (us *UserService) StopQuestionTimer(teamID int, questionID int) error {
	// Get the start time
	var startedAt time.Time
	getQuery := `SELECT started_at FROM question_timers WHERE team_id = ? AND question_id = ?`
	err := us.UserStore.DB.QueryRow(getQuery, teamID, questionID).Scan(&startedAt)
	if err != nil {
		log.Printf("Error getting start time for team %d, question %d: %v", teamID, questionID, err)
		return err
	}
	
	// Calculate time taken
	completedAt := time.Now()
	timeTaken := int(completedAt.Sub(startedAt).Seconds())
	
	// Update the timer record
	updateQuery := `UPDATE question_timers 
					SET completed_at = ?, time_taken_seconds = ? 
					WHERE team_id = ? AND question_id = ?`
	
	_, err = us.UserStore.DB.Exec(updateQuery, completedAt, timeTaken, teamID, questionID)
	if err != nil {
		log.Printf("Error stopping timer for team %d, question %d: %v", teamID, questionID, err)
		return err
	}
	
	log.Printf("Successfully stopped timer for team %d, question %d (Time: %d seconds)", teamID, questionID, timeTaken)
	return nil
}

// GetTotalSolveTime gets the total time taken by a team to solve all questions
func (us *UserService) GetTotalSolveTime(teamID int) (int, error) {
	query := `SELECT COALESCE(SUM(time_taken_seconds), 0) 
			  FROM question_timers 
			  WHERE team_id = ? AND completed_at IS NOT NULL`
	
	var totalTime int
	err := us.UserStore.DB.QueryRow(query, teamID).Scan(&totalTime)
	if err != nil {
		log.Printf("Error getting total solve time for team %d: %v", teamID, err)
		return 0, err
	}
	
	return totalTime, nil
}

// GetQuestionSolveTime gets the time taken to solve a specific question
func (us *UserService) GetQuestionSolveTime(teamID int, questionID int) (int, error) {
	query := `SELECT COALESCE(time_taken_seconds, 0) 
			  FROM question_timers 
			  WHERE team_id = ? AND question_id = ? AND completed_at IS NOT NULL`
	
	var timeTaken int
	err := us.UserStore.DB.QueryRow(query, teamID, questionID).Scan(&timeTaken)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		log.Printf("Error getting solve time for team %d, question %d: %v", teamID, questionID, err)
		return 0, err
	}
	
	return timeTaken, nil
}

// IsQuestionSolvedByAnyone checks if a question has been solved by any team
func (us *UserService) IsQuestionSolvedByAnyone(questionID int) (bool, error) {
	query := `SELECT COUNT(*) FROM team_completed_questions WHERE question_id = ?`
	var count int
	err := us.UserStore.DB.QueryRow(query, questionID).Scan(&count)
	if err != nil {
		log.Printf("Error checking if question %d is solved by anyone: %v", questionID, err)
		return false, err
	}
	return count > 0, nil
}

// CleanupStaleLocks removes all locks older than 10 seconds
// This should be called periodically to prevent abandoned locks
func (us *UserService) CleanupStaleLocks() error {
	query := `DELETE FROM question_locks WHERE locked_at < datetime('now', '-10 seconds')`
	
	result, err := us.UserStore.DB.Exec(query)
	if err != nil {
		log.Printf("Error cleaning up stale locks: %v", err)
		return err
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("Cleaned up %d stale question locks (timeout: 10 seconds)", rowsAffected)
	}
	
	return nil
}
