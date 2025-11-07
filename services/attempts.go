package services

import (
	"database/sql"
	"log"
	"time"

	"github.com/namishh/holmes/database"
)

type QuestionAttempt struct {
	TeamID        int       `json:"team_id"`
	QuestionID    int       `json:"question_id"`
	WrongAttempts int       `json:"wrong_attempts"`
	TotalPenalty  int       `json:"total_penalty"`
	LastAttemptAt time.Time `json:"last_attempt_at"`
}

// GetQuestionAttempts retrieves attempt info for a team on a specific question
func (us *UserService) GetQuestionAttempts(teamID int, questionID int) (*QuestionAttempt, error) {
	query := database.ConvertPlaceholders(`SELECT team_id, question_id, wrong_attempts, total_penalty, last_attempt_at 
			  FROM question_attempts 
			  WHERE team_id = ? AND question_id = ?`)
	
	var attempt QuestionAttempt
	err := us.UserStore.DB.QueryRow(query, teamID, questionID).Scan(
		&attempt.TeamID,
		&attempt.QuestionID,
		&attempt.WrongAttempts,
		&attempt.TotalPenalty,
		&attempt.LastAttemptAt,
	)
	
	if err == sql.ErrNoRows {
		// No attempts yet, return default
		return &QuestionAttempt{
			TeamID:        teamID,
			QuestionID:    questionID,
			WrongAttempts: 0,
			TotalPenalty:  0,
			LastAttemptAt: time.Now(),
		}, nil
	}
	
	if err != nil {
		log.Printf("Error getting attempts for team %d, question %d: %v", teamID, questionID, err)
		return nil, err
	}
	
	return &attempt, nil
}

// RecordWrongAttempt records a wrong attempt and calculates penalty based on question points
// Returns: (penalty amount, attempts left, error)
func (us *UserService) RecordWrongAttempt(teamID int, questionID int, questionPoints int) (int, int, error) {
	// Get current attempts
	attempt, err := us.GetQuestionAttempts(teamID, questionID)
	if err != nil {
		return 0, 0, err
	}
	
	// Calculate penalty as percentage of question points
	// 1st wrong: 0% penalty (warning)
	// 2nd wrong: 10% of question points
	// 3rd wrong: 30% of question points
	// 4th wrong: 50% of question points
	// 5th wrong: 70% of question points
	var penalty int
	switch attempt.WrongAttempts {
	case 0:
		penalty = 0 // First wrong - warning only
	case 1:
		penalty = (questionPoints * 10) / 100 // 10%
	case 2:
		penalty = (questionPoints * 30) / 100 // 30%
	case 3:
		penalty = (questionPoints * 50) / 100 // 50%
	case 4:
		penalty = (questionPoints * 70) / 100 // 70%
	default:
		penalty = 0
	}
	
	newAttempts := attempt.WrongAttempts + 1
	newTotalPenalty := attempt.TotalPenalty + penalty
	attemptsLeft := 5 - newAttempts
	
	// Insert or update the attempt record
	query := database.ConvertPlaceholders(`INSERT INTO question_attempts (team_id, question_id, wrong_attempts, total_penalty, last_attempt_at)
			  VALUES (?, ?, ?, ?, ?)
			  ON CONFLICT(team_id, question_id) DO UPDATE SET
			  wrong_attempts = ?,
			  total_penalty = ?,
			  last_attempt_at = ?`)
	
	now := time.Now()
	_, err = us.UserStore.DB.Exec(query, 
		teamID, questionID, newAttempts, newTotalPenalty, now,
		newAttempts, newTotalPenalty, now)
	
	if err != nil {
		log.Printf("Error recording wrong attempt for team %d, question %d: %v", teamID, questionID, err)
		return 0, 0, err
	}
	
	log.Printf("Recorded wrong attempt for team %d, question %d: attempts=%d, penalty=%d, total_penalty=%d", 
		teamID, questionID, newAttempts, penalty, newTotalPenalty)
	
	return penalty, attemptsLeft, nil
}

// IsQuestionExhausted checks if a team has exhausted all attempts for a question
func (us *UserService) IsQuestionExhausted(teamID int, questionID int) (bool, error) {
	attempt, err := us.GetQuestionAttempts(teamID, questionID)
	if err != nil {
		return false, err
	}
	
	return attempt.WrongAttempts >= 5, nil
}

// GetTotalPenalty gets the total penalty for a team across all questions
func (us *UserService) GetTotalPenalty(teamID int) (int, error) {
	query := database.ConvertPlaceholders(`SELECT COALESCE(SUM(total_penalty), 0) 
			  FROM question_attempts 
			  WHERE team_id = ?`)
	
	var totalPenalty int
	err := us.UserStore.DB.QueryRow(query, teamID).Scan(&totalPenalty)
	if err != nil {
		log.Printf("Error getting total penalty for team %d: %v", teamID, err)
		return 0, err
	}
	
	return totalPenalty, nil
}

// DeductPenaltyPoints deducts penalty points from team's score
func (us *UserService) DeductPenaltyPoints(teamID int, penalty int) error {
	if penalty <= 0 {
		return nil
	}
	
	query := database.ConvertPlaceholders(`UPDATE teams SET points = points - ? WHERE id = ?`)
	
	_, err := us.UserStore.DB.Exec(query, penalty, teamID)
	if err != nil {
		log.Printf("Error deducting penalty %d from team %d: %v", penalty, teamID, err)
		return err
	}
	
	log.Printf("Deducted %d penalty points from team %d", penalty, teamID)
	return nil
}
