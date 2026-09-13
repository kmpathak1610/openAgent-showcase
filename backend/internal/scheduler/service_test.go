package scheduler

import (
	"testing"
	"time"
)

func TestNextWeekly(t *testing.T) {
	// Monday 9am
	loc, _ := time.LoadLocation("UTC")
	now := time.Date(2024, 1, 1, 10, 0, 0, 0, loc) // Monday, Jan 1 2024 is Monday
	next := nextWeekly("0 9 * * 1", now)
	if next == nil { t.Fatal("no next") }
	if next.Weekday() != time.Monday { t.Fatalf("expected Monday, got %v", next.Weekday()) }
	if next.Hour() != 9 { t.Fatalf("expected 9, got %d", next.Hour()) }
	// If now is Monday 8am, next should be same day 9am
	now = time.Date(2024, 1, 1, 8, 0, 0, 0, loc)
	next = nextWeekly("0 9 * * 1", now)
	if next.Day() != 1 || next.Hour()!=9 { t.Fatalf("expected same Monday 9am, got %v", next) }
	// If now is Monday 10am, next should be next Monday
	now = time.Date(2024, 1, 1, 10, 0, 0, 0, loc)
	next = nextWeekly("0 9 * * 1", now)
	if next.Day() != 8 { t.Fatalf("expected next Monday 8th, got %v", next) }
}

func TestScheduler_OneTimeAndRecurring(t *testing.T) {
	// One-time scheduledAt should be parsed and set as next_run_at
	// This is tested via CreateTrigger mock, but we can test the helper
	if nextWeekly("not-a-cron", time.Now()) == nil {
		t.Fatal("should return next minute for invalid cron")
	}
}
