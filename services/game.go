package services

import (
	"log"
	"time"

	"github.com/namishh/holmes/database"
)

type QuestionWithStatus struct {
	ID               int    `json:"id"`
	Question         string `json:"question"`
	Answer           string `json:"answer"`
	Title            string `json:"title"`
	Points           int    `json:"points"`
	Solved           bool   `json:"solved"`
	Locked           bool   `json:"locked"`
	LockedByTeamID   int    `json:"locked_by_team_id"`
	LockedByName     string `json:"locked_by_name"`
	LockedByMe       bool   `json:"locked_by_me"`
	SolvedByAnyone   bool   `json:"solved_by_anyone"`
}

func (us *UserService) GetAllQuestionsWithStatus(userID int) ([]QuestionWithStatus, error) {
	query := `SELECT q.id, q.question, q.answer, q.title, q.points,
           CASE WHEN tcq_mine.team_id IS NOT NULL THEN 1 ELSE 0 END as solved,
           CASE WHEN ql.question_id IS NOT NULL THEN 1 ELSE 0 END as locked,
           COALESCE(ql.locked_by_team_id, 0) as locked_by_team_id,
           COALESCE(t.name, '') as locked_by_name,
           CASE WHEN ql.locked_by_team_id = $1 THEN 1 ELSE 0 END as locked_by_me,
           CASE WHEN tcq_any.question_id IS NOT NULL THEN 1 ELSE 0 END as solved_by_anyone
    FROM questions q
    LEFT JOIN team_completed_questions tcq_mine ON q.id = tcq_mine.question_id AND tcq_mine.team_id = $2
    LEFT JOIN question_locks ql ON q.id = ql.question_id
    LEFT JOIN teams t ON ql.locked_by_team_id = t.id
    LEFT JOIN (SELECT DISTINCT question_id FROM team_completed_questions) tcq_any ON q.id = tcq_any.question_id
    ORDER BY q.points ASC
    `

	rows, err := us.UserStore.DB.Query(query, userID, userID)
	if err != nil {
		log.Printf("Error querying questions with status: %v", err)
		return nil, err
	}
	defer rows.Close()

	var questions []QuestionWithStatus
	for rows.Next() {
		var q QuestionWithStatus
		var solved int
		var locked int
		var lockedByMe int
		var solvedByAnyone int
		err := rows.Scan(&q.ID, &q.Question, &q.Answer, &q.Title, &q.Points, &solved, &locked, &q.LockedByTeamID, &q.LockedByName, &lockedByMe, &solvedByAnyone)
		if err != nil {
			log.Printf("Error scanning question row: %v", err)
			return nil, err
		}
		q.Solved = solved == 1
		q.Locked = locked == 1
		q.LockedByMe = lockedByMe == 1
		q.SolvedByAnyone = solvedByAnyone == 1
		questions = append(questions, q)
	}

	if err = rows.Err(); err != nil {
		log.Printf("Error iterating question rows: %v", err)
		return nil, err
	}

	return questions, nil
}

func (us *UserService) HasCompletedAllQuestions(teamID int) (bool, error) {
	// Get total number of questions
	var totalQuestions int
	queryTotal := `SELECT COUNT(*) FROM questions`
	err := us.UserStore.DB.QueryRow(queryTotal).Scan(&totalQuestions)
	if err != nil {
		log.Printf("Error getting total question count: %v", err)
		return false, err
	}

	// Get number of completed questions for the team
	var completedCount int
	queryCompleted := database.ConvertPlaceholders(`SELECT COUNT(*) FROM team_completed_questions WHERE team_id = ?`)
	err = us.UserStore.DB.QueryRow(queryCompleted, teamID).Scan(&completedCount)
	if err != nil {
		log.Printf("Error getting completed question count for team %d: %v", teamID, err)
		return false, err
	}

	// Compare counts
	if totalQuestions == 0 {
		return false, nil
	}
	return completedCount >= totalQuestions, nil
}

func (us *UserService) MarkQuestionAsCompleted(userID, questionID int) error {
	query := database.ConvertPlaceholders(`INSERT OR IGNORE INTO team_completed_questions (team_id, question_id) VALUES (?, ?)`)
	_, err := us.UserStore.DB.Exec(query, userID, questionID)
	if err != nil {
		log.Printf("Error marking question %d as completed for user %d: %v", questionID, userID, err)
		return err
	}
	return nil
}

func (us *UserService) GetCompletedQuestions(userID int) ([]int, error) {
	query := database.ConvertPlaceholders(`SELECT question_id FROM team_completed_questions WHERE team_id = ?`)
	rows, err := us.UserStore.DB.Query(query, userID)
	if err != nil {
		log.Printf("Error getting completed questions for user %d: %v", userID, err)
		return nil, err
	}
	defer rows.Close()

	var completedQuestions []int
	for rows.Next() {
		var questionID int
		if err := rows.Scan(&questionID); err != nil {
			log.Printf("Error scanning completed question: %v", err)
			return nil, err
		}
		completedQuestions = append(completedQuestions, questionID)
	}

	return completedQuestions, nil
}

// Take a question ID, team ID and check if the question is solved by the team
// Return true if solved, false otherwise

func (us *UserService) IsQuestionSolvedByTeam(teamID, questionID int) (bool, error) {
	query := database.ConvertPlaceholders(`SELECT COUNT(*) FROM team_completed_questions WHERE team_id = ? AND question_id = ?`)
	var count int
	err := us.UserStore.DB.QueryRow(query, teamID, questionID).Scan(&count)
	if err != nil {
		log.Printf("Error checking if question %d is solved by team %d: %v", questionID, teamID, err)
		return false, err
	}
	return count > 0, nil
}

func (us *UserService) UpdateTeamLastAnsweredQuestion(teamID int) error {
	query := database.ConvertPlaceholders(`
    UPDATE teams
    SET last_answered_question = ?
    WHERE id = ?
    `)

	// Get current timestamp
	currentTime := time.Now()

	// Execute the update
	_, err := us.UserStore.DB.Exec(query, currentTime, teamID)
	if err != nil {
		log.Printf("Error updating last answered question for team %d: %v", teamID, err)
		return err
	}

	log.Printf("Update operation completed for team %d at %v", teamID, currentTime)
	return nil
}

type LeaderBoardUser struct {
	Username         string
	Points           int
	QuestionsSolved  int
	TotalTimeSeconds int
	TotalPenalty     int
	NetScore         int
}

func (us *UserService) GetLeaderbaord() ([]LeaderBoardUser, error) {
	// Updated query to include questions solved count, total solve time, and penalties
	// Using COUNT with CASE to properly count NULL values as 0
	// Sorting by: Net Score (DESC), Questions Solved (DESC), Time (ASC)
	stmt := `
		SELECT 
			t.name, 
			t.points,
			COUNT(CASE WHEN tcq.question_id IS NOT NULL THEN 1 END) as questions_solved,
			COALESCE(SUM(DISTINCT qt.time_taken_seconds), 0) as total_time,
			COALESCE(SUM(DISTINCT qa.total_penalty), 0) as total_penalty
		FROM teams t
		LEFT JOIN team_completed_questions tcq ON t.id = tcq.team_id
		LEFT JOIN question_timers qt ON t.id = qt.team_id AND qt.question_id = tcq.question_id AND qt.completed_at IS NOT NULL
		LEFT JOIN question_attempts qa ON t.id = qa.team_id
		GROUP BY t.id, t.name, t.points
		ORDER BY (t.points - COALESCE(SUM(DISTINCT qa.total_penalty), 0)) DESC, questions_solved DESC, total_time ASC, t.last_answered_question ASC;`
	
	rows, err := us.UserStore.DB.Query(stmt)
	if err != nil {
		log.Printf("Error fetching leaderboard: %v", err)
		return nil, err
	}
	defer rows.Close()

	var users []LeaderBoardUser

	for rows.Next() {
		var user LeaderBoardUser
		if err := rows.Scan(&user.Username, &user.Points, &user.QuestionsSolved, &user.TotalTimeSeconds, &user.TotalPenalty); err != nil {
			log.Printf("Error scanning leaderboard row: %v", err)
			return nil, err
		}
		user.NetScore = user.Points - user.TotalPenalty
		users = append(users, user)
	}

	return users, nil
}
