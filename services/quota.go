package services

import (
	"database/sql"
	"log"
	"time"

	"github.com/namishh/holmes/database"
)

const (
	QuotaLimit       = 10                // 10 questions per slot
	SlotDuration     = 10 * time.Hour    // 10 hours per slot
)

type QuotaSlot struct {
	TeamID              int       `json:"team_id"`
	CurrentSlotStart    time.Time `json:"current_slot_start"`
	QuestionsSolvedInSlot int     `json:"questions_solved_in_slot"`
}

// GetQuotaSlot retrieves the current quota slot for a team
func (us *UserService) GetQuotaSlot(teamID int) (*QuotaSlot, error) {
	query := database.ConvertPlaceholders(`SELECT team_id, current_slot_start, questions_solved_in_slot 
			  FROM team_quota_slots 
			  WHERE team_id = ?`)
	
	var slot QuotaSlot
	err := us.UserStore.DB.QueryRow(query, teamID).Scan(
		&slot.TeamID,
		&slot.CurrentSlotStart,
		&slot.QuestionsSolvedInSlot,
	)
	
	if err == sql.ErrNoRows {
		// No slot exists, create a new one
		return us.CreateQuotaSlot(teamID)
	}
	
	if err != nil {
		log.Printf("Error getting quota slot for team %d: %v", teamID, err)
		return nil, err
	}
	
	// Check if the current slot has expired (10 hours passed)
	if time.Since(slot.CurrentSlotStart) >= SlotDuration {
		// Reset the slot
		return us.ResetQuotaSlot(teamID)
	}
	
	return &slot, nil
}

// CreateQuotaSlot creates a new quota slot for a team
func (us *UserService) CreateQuotaSlot(teamID int) (*QuotaSlot, error) {
	now := time.Now()
	query := database.ConvertPlaceholders(`INSERT INTO team_quota_slots (team_id, current_slot_start, questions_solved_in_slot)
			  VALUES (?, ?, 0)`)
	
	_, err := us.UserStore.DB.Exec(query, teamID, now)
	if err != nil {
		log.Printf("Error creating quota slot for team %d: %v", teamID, err)
		return nil, err
	}
	
	return &QuotaSlot{
		TeamID:              teamID,
		CurrentSlotStart:    now,
		QuestionsSolvedInSlot: 0,
	}, nil
}

// ResetQuotaSlot resets the quota slot for a team (starts a new 10-hour window)
func (us *UserService) ResetQuotaSlot(teamID int) (*QuotaSlot, error) {
	now := time.Now()
	query := database.ConvertPlaceholders(`UPDATE team_quota_slots 
			  SET current_slot_start = ?, questions_solved_in_slot = 0 
			  WHERE team_id = ?`)
	
	_, err := us.UserStore.DB.Exec(query, now, teamID)
	if err != nil {
		log.Printf("Error resetting quota slot for team %d: %v", teamID, err)
		return nil, err
	}
	
	log.Printf("Reset quota slot for team %d at %v", teamID, now)
	return &QuotaSlot{
		TeamID:              teamID,
		CurrentSlotStart:    now,
		QuestionsSolvedInSlot: 0,
	}, nil
}

// IncrementQuotaCount increments the questions solved count in the current slot
func (us *UserService) IncrementQuotaCount(teamID int) error {
	query := database.ConvertPlaceholders(`UPDATE team_quota_slots 
			  SET questions_solved_in_slot = questions_solved_in_slot + 1 
			  WHERE team_id = ?`)
	
	_, err := us.UserStore.DB.Exec(query, teamID)
	if err != nil {
		log.Printf("Error incrementing quota count for team %d: %v", teamID, err)
		return err
	}
	
	log.Printf("Incremented quota count for team %d", teamID)
	return nil
}

// CanSolveQuestion checks if a team can solve another question based on quota
func (us *UserService) CanSolveQuestion(teamID int) (bool, *QuotaSlot, error) {
	slot, err := us.GetQuotaSlot(teamID)
	if err != nil {
		return false, nil, err
	}
	
	// Check if quota is exhausted
	if slot.QuestionsSolvedInSlot >= QuotaLimit {
		return false, slot, nil
	}
	
	return true, slot, nil
}

// GetTimeUntilQuotaReset returns the duration until the quota resets
func (us *UserService) GetTimeUntilQuotaReset(teamID int) (time.Duration, error) {
	slot, err := us.GetQuotaSlot(teamID)
	if err != nil {
		return 0, err
	}
	
	elapsed := time.Since(slot.CurrentSlotStart)
	remaining := SlotDuration - elapsed
	
	if remaining < 0 {
		return 0, nil
	}
	
	return remaining, nil
}

// GetActualCompletedQuestionsCount returns the actual number of questions completed by the team
// This is different from QuestionsSolvedInSlot which is quota-based and resets every 10 hours
func (us *UserService) GetActualCompletedQuestionsCount(teamID int) (int, error) {
	query := database.ConvertPlaceholders(`SELECT COUNT(*) FROM team_completed_questions WHERE team_id = ?`)
	
	var count int
	err := us.UserStore.DB.QueryRow(query, teamID).Scan(&count)
	if err != nil {
		log.Printf("Error getting completed questions count for team %d: %v", teamID, err)
		return 0, err
	}
	
	return count, nil
}
