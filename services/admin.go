package services

import (
	"log"
	"os"

	"github.com/namishh/holmes/database"
)

// AdminUnlockQuestion allows admin to unlock a solved question so other users can attempt it
// This PRESERVES existing completions and only clears locks/timers/attempts for non-solvers
func (us *UserService) AdminUnlockQuestion(questionID int) error {
	// DO NOT delete from team_completed_questions - keep existing solves!
	
	// Remove any active locks on this question
	lockQuery := database.ConvertPlaceholders(`DELETE FROM question_locks WHERE question_id = ?`)
	result, err := us.UserStore.DB.Exec(lockQuery, questionID)
	if err != nil {
		log.Printf("Error removing locks for question %d: %v", questionID, err)
		return err
	}
	locksRemoved, _ := result.RowsAffected()
	
	// Reset question timers ONLY for teams who haven't solved it
	timerQuery := database.ConvertPlaceholders(`
		DELETE FROM question_timers 
		WHERE question_id = ? 
		AND team_id NOT IN (SELECT team_id FROM team_completed_questions WHERE question_id = ?)`)
	result, err = us.UserStore.DB.Exec(timerQuery, questionID, questionID)
	if err != nil {
		log.Printf("Error removing timers for question %d: %v", questionID, err)
	}
	timersRemoved, _ := result.RowsAffected()
	
	// Reset wrong attempts ONLY for teams who haven't solved it
	attemptsQuery := database.ConvertPlaceholders(`
		DELETE FROM question_attempts 
		WHERE question_id = ? 
		AND team_id NOT IN (SELECT team_id FROM team_completed_questions WHERE question_id = ?)`)
	result, err = us.UserStore.DB.Exec(attemptsQuery, questionID, questionID)
	if err != nil {
		log.Printf("Error removing attempts for question %d: %v", questionID, err)
	}
	attemptsRemoved, _ := result.RowsAffected()
	
	log.Printf("Admin unlocked question %d for other users (locks: %d, timers: %d, attempts: %d). Existing solves preserved.", 
		questionID, locksRemoved, timersRemoved, attemptsRemoved)
	return nil
}

// GetSolvedQuestions returns all questions that have been solved by any user
func (us *UserService) GetSolvedQuestions() ([]QuestionWithSolvers, error) {
	var query string
	if os.Getenv("DATABASE_URL") != "" {
		// PostgreSQL syntax - use STRING_AGG instead of GROUP_CONCAT
		query = `
			SELECT 
				q.id, 
				q.title, 
				q.points,
				STRING_AGG(t.name, ', ') as solvers
			FROM questions q
			INNER JOIN team_completed_questions tcq ON q.id = tcq.question_id
			INNER JOIN teams t ON tcq.team_id = t.id
			GROUP BY q.id, q.title, q.points
			ORDER BY q.points ASC
		`
	} else {
		// SQLite syntax
		query = `
			SELECT 
				q.id, 
				q.title, 
				q.points,
				GROUP_CONCAT(t.name, ', ') as solvers
			FROM questions q
			INNER JOIN team_completed_questions tcq ON q.id = tcq.question_id
			INNER JOIN teams t ON tcq.team_id = t.id
			GROUP BY q.id, q.title, q.points
			ORDER BY q.points ASC
		`
	}
	
	rows, err := us.UserStore.DB.Query(query)
	if err != nil {
		log.Printf("Error getting solved questions: %v", err)
		return nil, err
	}
	defer rows.Close()
	
	var questions []QuestionWithSolvers
	for rows.Next() {
		var q QuestionWithSolvers
		err := rows.Scan(&q.ID, &q.Title, &q.Points, &q.SolvedBy)
		if err != nil {
			log.Printf("Error scanning solved question: %v", err)
			return nil, err
		}
		questions = append(questions, q)
	}
	
	return questions, nil
}

type QuestionWithSolvers struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Points   int    `json:"points"`
	SolvedBy string `json:"solved_by"`
}
