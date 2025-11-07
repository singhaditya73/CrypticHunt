package services

import (
	"fmt"
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/namishh/holmes/database"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID        int    `json:"id"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Username  string `json:"username"`
	Points    int    `json:"points"`
	CreatedAt string `json:"created_at"`
}

type UserService struct {
	User        User
	UserStore   database.DatabaseStore
	MinioClient *minio.Client
}

func NewUserService(user User, userStore database.DatabaseStore, mini *minio.Client) *UserService {
	return &UserService{
		User:        user,
		UserStore:   userStore,
		MinioClient: mini,
	}
}

func (us *UserService) CreateUser(u User) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	stmt := `INSERT INTO teams (email, password, name, points) VALUES ($1, $2, $3, 0)`

	_, err = us.UserStore.DB.Exec(stmt, u.Email, string(hashedPassword), u.Username)
	return err
}

func (us *UserService) CheckUsername(usr string) (User, error) {
	query := `SELECT id, email, password, name, points FROM teams
		WHERE name = $1`

	us.User.Username = usr
	err := us.UserStore.DB.QueryRow(query, us.User.Username).Scan(
		&us.User.ID,
		&us.User.Email,
		&us.User.Password,
		&us.User.Username,
		&us.User.Points,
	)
	if err != nil {
		return User{}, err
	}

	return us.User, nil
}

func (us *UserService) CheckEmail(email string) (User, error) {
	query := `SELECT id, email, password, name FROM teams
		WHERE email = $1`

	us.User.Email = email
	err := us.UserStore.DB.QueryRow(query, us.User.Email).Scan(
		&us.User.ID,
		&us.User.Email,
		&us.User.Password,
		&us.User.Username,
	)
	if err != nil {
		return User{}, err
	}

	return us.User, nil
}

func (us *UserService) GetAllUsers() ([]User, error) {
	query := `SELECT id, email, name, points FROM teams`
	users := make([]User, 0)
	stmt, err := us.UserStore.DB.Prepare(query)
	if err != nil {
		return users, err
	}

	defer stmt.Close()

	rows, err := stmt.Query()
	if err != nil {
		return users, err
	}

	for rows.Next() {
		var u User
		err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.Points)
		if err != nil {
			return users, err
		}

		users = append(users, u)
	}

	return users, nil
}

func (us *UserService) DeleteTeam(id int) error {
	log.Printf("Attempting to delete team with ID: %d", id)
	
	// Delete all related records first to avoid foreign key constraints
	
	// 1. Delete team_completed_questions
	query := database.ConvertPlaceholders(`DELETE FROM team_completed_questions WHERE team_id = ?`)
	_, err := us.UserStore.DB.Exec(query, id)
	if err != nil {
		log.Printf("Error deleting completed questions for team %d: %v", id, err)
		return fmt.Errorf("failed to delete completed questions: %v", err)
	}
	
	// 2. Delete question_locks
	query = database.ConvertPlaceholders(`DELETE FROM question_locks WHERE locked_by_team_id = ?`)
	_, err = us.UserStore.DB.Exec(query, id)
	if err != nil {
		log.Printf("Error deleting locks for team %d: %v", id, err)
		return fmt.Errorf("failed to delete question locks: %v", err)
	}
	
	// 3. Delete question_timers
	query = database.ConvertPlaceholders(`DELETE FROM question_timers WHERE team_id = ?`)
	_, err = us.UserStore.DB.Exec(query, id)
	if err != nil {
		log.Printf("Error deleting timers for team %d: %v", id, err)
		return fmt.Errorf("failed to delete question timers: %v", err)
	}
	
	// 4. Delete question_attempts
	query = database.ConvertPlaceholders(`DELETE FROM question_attempts WHERE team_id = ?`)
	_, err = us.UserStore.DB.Exec(query, id)
	if err != nil {
		log.Printf("Error deleting attempts for team %d: %v", id, err)
		return fmt.Errorf("failed to delete question attempts: %v", err)
	}
	
	// 5. Delete team_hint_unlocked
	query = database.ConvertPlaceholders(`DELETE FROM team_hint_unlocked WHERE team_id = ?`)
	_, err = us.UserStore.DB.Exec(query, id)
	if err != nil {
		log.Printf("Error deleting hint unlocks for team %d: %v", id, err)
		return fmt.Errorf("failed to delete hint unlocks: %v", err)
	}
	
	// 6. Delete team_quota_slots
	query = database.ConvertPlaceholders(`DELETE FROM team_quota_slots WHERE team_id = ?`)
	_, err = us.UserStore.DB.Exec(query, id)
	if err != nil {
		log.Printf("Error deleting quota slots for team %d: %v", id, err)
		return fmt.Errorf("failed to delete quota slots: %v", err)
	}
	
	// 7. Finally, delete the team itself
	query = database.ConvertPlaceholders(`DELETE FROM teams WHERE id = ?`)
	result, err := us.UserStore.DB.Exec(query, id)
	if err != nil {
		log.Printf("Error deleting team %d: %v", id, err)
		return fmt.Errorf("failed to delete team: %v", err)
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("Team %d not found", id)
		return fmt.Errorf("team not found")
	}
	
	log.Printf("Successfully deleted team %d and all related records", id)
	return nil
}

// PingDB checks if the database connection is alive
func (us *UserService) PingDB() error {
	return us.UserStore.DB.Ping()
}

// GetDBStats returns database connection pool statistics
func (us *UserService) GetDBStats() database.DBStats {
	stats := us.UserStore.DB.Stats()
	return database.DBStats{
		MaxOpenConnections: stats.MaxOpenConnections,
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
		WaitCount:          stats.WaitCount,
		WaitDuration:       stats.WaitDuration,
		MaxIdleClosed:      stats.MaxIdleClosed,
		MaxIdleTimeClosed:  stats.MaxIdleTimeClosed,
		MaxLifetimeClosed:  stats.MaxLifetimeClosed,
	}
}
