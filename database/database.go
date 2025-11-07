package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

type DatabaseStore struct {
	DB *sql.DB
}

// DBStats represents database connection pool statistics
type DBStats struct {
	MaxOpenConnections int
	OpenConnections    int
	InUse              int
	Idle               int
	WaitCount          int64
	WaitDuration       time.Duration
	MaxIdleClosed      int64
	MaxIdleTimeClosed  int64
	MaxLifetimeClosed  int64
}

func GetConnection(dbName string) (*sql.DB, error) {
	var db *sql.DB
	var err error
	
	// Check if DATABASE_URL is set (PostgreSQL)
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL != "" {
		// Use PostgreSQL
		db, err = sql.Open("postgres", databaseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to PostgreSQL: %s", err)
		}
		log.Println("Using PostgreSQL database")
	} else {
		// Use SQLite for local development
		db, err = sql.Open("sqlite3", dbName)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to SQLite: %s", err)
		}
		log.Println("Using SQLite database")
	}

	// Configure connection pool for optimal performance
	db.SetMaxOpenConns(50)           // Maximum number of open connections
	db.SetMaxIdleConns(25)            // Maximum number of idle connections
	db.SetConnMaxLifetime(5 * time.Minute) // Maximum lifetime of a connection
	db.SetConnMaxIdleTime(2 * time.Minute) // Maximum idle time

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %s", err)
	}

	log.Println("Connected Successfully to the Database with optimized pool settings")

	return db, nil
}

func CreateMigrations(DBName string, DB *sql.DB) error {
	// Detect if using PostgreSQL or SQLite
	isPostgres := os.Getenv("DATABASE_URL") != ""
	
	// Helper function to convert SQL syntax
	autoIncrement := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if isPostgres {
		autoIncrement = "SERIAL PRIMARY KEY"
	}
	
	currentTimestamp := "CURRENT_TIMESTAMP"
	if isPostgres {
		currentTimestamp = "NOW()"
	}

	stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS teams (
		id %s,
		email VARCHAR(255) NOT NULL,
    	points INT DEFAULT 0,
		password VARCHAR(255) NOT NULL,
		name VARCHAR(255) UNIQUE NOT NULL,
		last_answered_question TIMESTAMP DEFAULT %s,
		created_at TIMESTAMP DEFAULT %s
		);
	`, autoIncrement, currentTimestamp, currentTimestamp)

	_, err := DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create teams table: %s", err)
	}

	stmt = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS questions (
		id %s,
    	question TEXT,
     	answer TEXT,
      	title TEXT,
       	points INT
	);`, autoIncrement)

	_, err = DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create questions table: %s", err)
	}

	stmt = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS hints  (
		id %s,
     	hint TEXT,
      	worth INT,
       parent_question_id INT,
       	FOREIGN KEY (parent_question_id) REFERENCES questions(id)
	);`, autoIncrement)

	_, err = DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create hints table: %s", err)
	}

	stmt = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS images (
		id %s,
    	path TEXT,
     	parent_question_id INTEGER,
     	FOREIGN KEY (parent_question_id) REFERENCES questions(id)
	);`, autoIncrement)

	_, err = DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create images table: %s", err)
	}

	stmt = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS audios (
		id %s,
    	path TEXT,
     	parent_question_id INTEGER,
     	FOREIGN KEY(parent_question_id) REFERENCES questions(id)
	);`, autoIncrement)

	_, err = DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create audios table: %s", err)
	}

	stmt = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS videos (
		id %s,
    	path TEXT,
     	parent_question_id INTEGER,
     	FOREIGN KEY(parent_question_id) REFERENCES questions(id)
	);`, autoIncrement)

	_, err = DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create videos table: %s", err)
	}

	stmt = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS team_completed_questions (
    team_id INTEGER,
    question_id INTEGER,
    completed_at TIMESTAMP DEFAULT %s,
    PRIMARY KEY (team_id, question_id),
    FOREIGN KEY (team_id) REFERENCES teams(id),
    FOREIGN KEY (question_id) REFERENCES questions(id)
    );`, currentTimestamp)

	_, err = DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create team_completed_questions table: %s", err)
	}

	stmt = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS team_hint_unlocked (
    team_id INTEGER,
    hint_id INTEGER,
    unlocked_at TIMESTAMP DEFAULT %s,
    PRIMARY KEY (team_id, hint_id),
    FOREIGN KEY (team_id) REFERENCES teams(id),
    FOREIGN KEY (hint_id) REFERENCES hints(id)
    );`, currentTimestamp)

	_, err = DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create team_hint_unlocked table: %s", err)
	}

	// Table to track locked questions
	stmt = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS question_locks (
    question_id INTEGER PRIMARY KEY,
    locked_by_team_id INTEGER NOT NULL,
    locked_at TIMESTAMP DEFAULT %s,
    FOREIGN KEY (question_id) REFERENCES questions(id),
    FOREIGN KEY (locked_by_team_id) REFERENCES teams(id)
    );`, currentTimestamp)

	_, err = DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create question_locks table: %s", err)
	}

	// Table to track question timers and solve times
	stmt = `CREATE TABLE IF NOT EXISTS question_timers (
    team_id INTEGER,
    question_id INTEGER,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    time_taken_seconds INTEGER,
    PRIMARY KEY (team_id, question_id),
    FOREIGN KEY (team_id) REFERENCES teams(id),
    FOREIGN KEY (question_id) REFERENCES questions(id)
    );`

	_, err = DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create question_timers table: %s", err)
	}

	// Table to track wrong attempts and penalties
	stmt = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS question_attempts (
    team_id INTEGER,
    question_id INTEGER,
    wrong_attempts INTEGER DEFAULT 0,
    total_penalty INTEGER DEFAULT 0,
    last_attempt_at TIMESTAMP DEFAULT %s,
    PRIMARY KEY (team_id, question_id),
    FOREIGN KEY (team_id) REFERENCES teams(id),
    FOREIGN KEY (question_id) REFERENCES questions(id)
    );`, currentTimestamp)

	_, err = DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create question_attempts table: %s", err)
	}

	// Table to track question solving quota and time slots
	stmt = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS team_quota_slots (
    team_id INTEGER PRIMARY KEY,
    current_slot_start TIMESTAMP DEFAULT %s,
    questions_solved_in_slot INTEGER DEFAULT 0,
    FOREIGN KEY (team_id) REFERENCES teams(id)
    );`, currentTimestamp)

	_, err = DB.Exec(stmt)
	if err != nil {
		return fmt.Errorf("Failed to create team_quota_slots table: %s", err)
	}

	// Create indexes for performance optimization
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_question_locks_question_id ON question_locks(question_id);`,
		`CREATE INDEX IF NOT EXISTS idx_question_timers_team_question ON question_timers(team_id, question_id);`,
		`CREATE INDEX IF NOT EXISTS idx_question_attempts_team_question ON question_attempts(team_id, question_id);`,
		`CREATE INDEX IF NOT EXISTS idx_team_completed_questions ON team_completed_questions(team_id, question_id);`,
		`CREATE INDEX IF NOT EXISTS idx_team_hint_unlocked ON team_hint_unlocked(team_id, hint_id);`,
	}

	for _, indexStmt := range indexes {
		// For PostgreSQL, we need to handle existing indexes differently
		if isPostgres {
			indexStmt = strings.Replace(indexStmt, "CREATE INDEX IF NOT EXISTS", "CREATE INDEX IF NOT EXISTS", 1)
		}
		if _, err := DB.Exec(indexStmt); err != nil {
			log.Printf("Warning: Failed to create index: %v", err)
		}
	}

	log.Println("Database indexes created successfully")

	return nil
}

func NewDatabaseStore(path string) (DatabaseStore, error) {
	DB, err := GetConnection(path)
	if err != nil {
		return DatabaseStore{}, err
	}

	if err := CreateMigrations(path, DB); err != nil {
		return DatabaseStore{}, err
	}

	return DatabaseStore{DB: DB}, nil
}
