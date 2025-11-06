package services

import "log"

// AdminUnlockQuestion allows admin to unlock a solved question so other users can attempt it
func (us *UserService) AdminUnlockQuestion(questionID int) error {
	// Remove all completion records for this question
	query := `DELETE FROM team_completed_questions WHERE question_id = ?`
	
	_, err := us.UserStore.DB.Exec(query, questionID)
	if err != nil {
		log.Printf("Error unlocking question %d: %v", questionID, err)
		return err
	}
	
	// Also remove any active locks on this question
	lockQuery := `DELETE FROM question_locks WHERE question_id = ?`
	_, err = us.UserStore.DB.Exec(lockQuery, questionID)
	if err != nil {
		log.Printf("Error removing locks for question %d: %v", questionID, err)
		return err
	}
	
	log.Printf("Admin unlocked question %d for all users", questionID)
	return nil
}

// GetSolvedQuestions returns all questions that have been solved by any user
func (us *UserService) GetSolvedQuestions() ([]QuestionWithSolvers, error) {
	query := `
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
