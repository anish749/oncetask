package oncetask

import (
	"testing"
	"time"
)

func TestFindOccurrence(t *testing.T) {
	jan1 := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	jan2 := time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC)
	jan10 := time.Date(2025, 1, 10, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		recurrence    *Recurrence
		referenceTime time.Time
		inclusive     bool
		wantTime      time.Time
		wantExhausted bool
		wantErr       bool
	}{
		{
			name: "inclusive includes exact time",
			recurrence: &Recurrence{
				RRule:   "FREQ=DAILY",
				DTStart: jan1.Format(time.RFC3339),
			},
			referenceTime: jan1,
			inclusive:     true,
			wantTime:      jan1,
			wantExhausted: false,
		},
		{
			name: "exclusive excludes exact time",
			recurrence: &Recurrence{
				RRule:   "FREQ=DAILY",
				DTStart: jan1.Format(time.RFC3339),
			},
			referenceTime: jan1,
			inclusive:     false,
			wantTime:      jan2,
			wantExhausted: false,
		},
		{
			name: "inclusive with ExDate skips excluded date",
			recurrence: &Recurrence{
				RRule:   "FREQ=DAILY",
				DTStart: jan1.Format(time.RFC3339),
				ExDates: []string{jan1.Format(time.RFC3339)},
			},
			referenceTime: jan1,
			inclusive:     true,
			wantTime:      jan2,
			wantExhausted: false,
		},
		{
			name: "exhausted when no more occurrences",
			recurrence: &Recurrence{
				RRule:   "FREQ=DAILY;COUNT=3",
				DTStart: jan1.Format(time.RFC3339),
			},
			referenceTime: jan10,
			inclusive:     true,
			wantExhausted: true,
		},
		{
			name: "invalid RRule returns error",
			recurrence: &Recurrence{
				RRule:   "INVALID",
				DTStart: jan1.Format(time.RFC3339),
			},
			referenceTime: jan1,
			inclusive:     true,
			wantErr:       true,
		},
		{
			name: "invalid DTStart returns error",
			recurrence: &Recurrence{
				RRule:   "FREQ=DAILY",
				DTStart: "not-a-timestamp",
			},
			referenceTime: jan1,
			inclusive:     true,
			wantErr:       true,
		},
		{
			name: "missing DTStart returns error",
			recurrence: &Recurrence{
				RRule:   "FREQ=DAILY",
				DTStart: "",
			},
			referenceTime: jan1,
			inclusive:     true,
			wantErr:       true,
		},
		{
			name: "missing RRule returns error",
			recurrence: &Recurrence{
				RRule:   "",
				DTStart: jan1.Format(time.RFC3339),
			},
			referenceTime: jan1,
			inclusive:     true,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, exhausted, err := findOccurrence(tt.recurrence, tt.referenceTime, tt.inclusive)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if exhausted != tt.wantExhausted {
				t.Errorf("exhausted = %v, want %v", exhausted, tt.wantExhausted)
			}

			if !tt.wantExhausted && !result.Equal(tt.wantTime) {
				t.Errorf("result = %v, want %v", result, tt.wantTime)
			}
		})
	}
}

func TestCalculateNextOccurrence(t *testing.T) {
	jan1 := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	jan2 := time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC)
	jan3 := time.Date(2025, 1, 3, 9, 0, 0, 0, time.UTC)
	jan5 := time.Date(2025, 1, 5, 9, 0, 0, 0, time.UTC)
	jan6 := time.Date(2025, 1, 6, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		recurrence    *Recurrence
		after         time.Time
		wantTime      time.Time
		wantExhausted bool
	}{
		{
			name: "finds next day for daily recurrence",
			recurrence: &Recurrence{
				RRule:   "FREQ=DAILY",
				DTStart: jan1.Format(time.RFC3339),
			},
			after:    jan1,
			wantTime: jan2,
		},
		{
			name: "finds correct next after arbitrary time",
			recurrence: &Recurrence{
				RRule:   "FREQ=DAILY",
				DTStart: jan1.Format(time.RFC3339),
			},
			after:    jan5,
			wantTime: jan6,
		},
		{
			name: "exhausted after last occurrence",
			recurrence: &Recurrence{
				RRule:   "FREQ=DAILY;COUNT=3",
				DTStart: jan1.Format(time.RFC3339),
			},
			after:         jan3,
			wantExhausted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, exhausted, err := calculateNextOccurrence(tt.recurrence, tt.after)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if exhausted != tt.wantExhausted {
				t.Errorf("exhausted = %v, want %v", exhausted, tt.wantExhausted)
			}

			if !tt.wantExhausted && !result.Equal(tt.wantTime) {
				t.Errorf("result = %v, want %v", result, tt.wantTime)
			}
		})
	}
}
