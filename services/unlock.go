package services

import (
	"log"

	"github.com/namishh/holmes/database"
)

type SolvedQuestionInfo struct {
	QuestionID    int    `json:"question_id"`
	QuestionTitle string `json:"question_title"`
	Points        int    `json:"points"`
	SolvedByTeam  string `json:"solved_by_team"`
	TeamID        int    `json:"team_id"`
	SolvedAt      string `json:"solved_at"`
}

// GetAllSolvedQuestions retrieves all questions that have been solved by any team
func (us *UserService) GetAllSolvedQuestions() ([]SolvedQuestionInfo, error) {
	query := `
		SELECT 
			q.id,
			q.title,
			q.points,
			t.name,
			tcq.team_id,
			tcq.completed_at
		FROM questions q
		INNER JOIN team_completed_questions tcq ON q.id = tcq.question_id
		INNER JOIN teams t ON tcq.team_id = t.id
		ORDER BY tcq.completed_at DESC`
	
	rows, err := us.UserStore.DB.Query(query)
	if err != nil {
		log.Printf("Error getting all solved questions: %v", err)
		return nil, err
	}
	defer rows.Close()
	
	var solvedQuestions []SolvedQuestionInfo
	for rows.Next() {
		var sq SolvedQuestionInfo
		err := rows.Scan(
			&sq.QuestionID,
			&sq.QuestionTitle,
			&sq.Points,
			&sq.SolvedByTeam,
			&sq.TeamID,
			&sq.SolvedAt,
		)
		if err != nil {
			log.Printf("Error scanning solved question: %v", err)
			continue
		}
		solvedQuestions = append(solvedQuestions, sq)
	}
	
	return solvedQuestions, nil
}

// UnlockSolvedQuestion removes a question from the completed list for a specific team
func (us *UserService) UnlockSolvedQuestion(questionID int, teamID int) error {
	// Remove from team_completed_questions
	query := database.ConvertPlaceholders(`DELETE FROM team_completed_questions WHERE question_id = ? AND team_id = ?`)
	
	result, err := us.UserStore.DB.Exec(query, questionID, teamID)
	if err != nil {
		log.Printf("Error unlocking question %d for team %d: %v", questionID, teamID, err)
		return err
	}
	
	rowsAffected, _ := result.RowsAffected()
	
	// Remove any locks by this team on this question
	lockQuery := database.ConvertPlaceholders(`DELETE FROM question_locks WHERE question_id = ? AND locked_by_team_id = ?`)
	_, err = us.UserStore.DB.Exec(lockQuery, questionID, teamID)
	if err != nil {
		log.Printf("Error removing lock for question %d, team %d: %v", questionID, teamID, err)
	}
	
	// Reset timer for this team on this question
	timerQuery := database.ConvertPlaceholders(`DELETE FROM question_timers WHERE question_id = ? AND team_id = ?`)
	_, err = us.UserStore.DB.Exec(timerQuery, questionID, teamID)
	if err != nil {
		log.Printf("Error removing timer for question %d, team %d: %v", questionID, teamID, err)
	}
	
	// Reset attempts for this team on this question
	attemptsQuery := database.ConvertPlaceholders(`DELETE FROM question_attempts WHERE question_id = ? AND team_id = ?`)
	_, err = us.UserStore.DB.Exec(attemptsQuery, questionID, teamID)
	if err != nil {
		log.Printf("Error removing attempts for question %d, team %d: %v", questionID, teamID, err)
	}
	
	log.Printf("Successfully unlocked question %d for team %d (%d completion records removed)", questionID, teamID, rowsAffected)
	return nil
}

// UnlockAllSolvedQuestions unlocks a question for teams who HAVEN'T solved it yet
// Does NOT remove existing completions - only clears locks, timers, and attempts
// This allows other users to attempt the question while preserving existing solves
func (us *UserService) UnlockAllSolvedQuestions(questionID int) error {
	// DO NOT delete from team_completed_questions - keep existing solves!
	
	// Remove ANY locks on this question (so others can attempt)
	lockQuery := database.ConvertPlaceholders(`DELETE FROM question_locks WHERE question_id = ?`)
	result, err := us.UserStore.DB.Exec(lockQuery, questionID)
	if err != nil {
		log.Printf("Error removing locks for question %d: %v", questionID, err)
		return err
	}
	locksRemoved, _ := result.RowsAffected()
	
	// Reset timers ONLY for teams who haven't completed it
	timerQuery := database.ConvertPlaceholders(`
		DELETE FROM question_timers 
		WHERE question_id = ? 
		AND team_id NOT IN (SELECT team_id FROM team_completed_questions WHERE question_id = ?)`)
	result, err = us.UserStore.DB.Exec(timerQuery, questionID, questionID)
	if err != nil {
		log.Printf("Error removing timers for question %d: %v", questionID, err)
	}
	timersRemoved, _ := result.RowsAffected()
	
	// Reset attempts ONLY for teams who haven't completed it
	attemptsQuery := database.ConvertPlaceholders(`
		DELETE FROM question_attempts 
		WHERE question_id = ? 
		AND team_id NOT IN (SELECT team_id FROM team_completed_questions WHERE question_id = ?)`)
	result, err = us.UserStore.DB.Exec(attemptsQuery, questionID, questionID)
	if err != nil {
		log.Printf("Error removing attempts for question %d: %v", questionID, err)
	}
	attemptsRemoved, _ := result.RowsAffected()
	
	log.Printf("Unlocked question %d for other users (locks: %d, timers: %d, attempts: %d cleared). Existing solves preserved.", 
		questionID, locksRemoved, timersRemoved, attemptsRemoved)
	return nil
}
