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

// UnlockSolvedQuestion removes a question from the completed list, allowing others to solve it
func (us *UserService) UnlockSolvedQuestion(questionID int, teamID int) error {
	// Remove from team_completed_questions
	query := database.ConvertPlaceholders(`DELETE FROM team_completed_questions WHERE question_id = ? AND team_id = ?`)
	
	_, err := us.UserStore.DB.Exec(query, questionID, teamID)
	if err != nil {
		log.Printf("Error unlocking question %d for team %d: %v", questionID, teamID, err)
		return err
	}
	
	log.Printf("Successfully unlocked question %d (previously solved by team %d)", questionID, teamID)
	return nil
}

// UnlockAllSolvedQuestions removes all completions for a specific question
func (us *UserService) UnlockAllSolvedQuestions(questionID int) error {
	query := database.ConvertPlaceholders(`DELETE FROM team_completed_questions WHERE question_id = ?`)
	
	result, err := us.UserStore.DB.Exec(query, questionID)
	if err != nil {
		log.Printf("Error unlocking all completions for question %d: %v", questionID, err)
		return err
	}
	
	rows, _ := result.RowsAffected()
	log.Printf("Successfully unlocked question %d (%d teams affected)", questionID, rows)
	return nil
}
