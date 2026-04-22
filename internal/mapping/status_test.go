package mapping

import "testing"

func TestFromListName(t *testing.T) {
	tests := []struct {
		input   string
		want    Status
		wantErr bool
	}{
		// Completed
		{"played", StatusCompleted, false},
		{"Played", StatusCompleted, false},
		{"PLAYED", StatusCompleted, false},
		{"completed", StatusCompleted, false},
		{"finished", StatusCompleted, false},
		// In progress
		{"playing", StatusInProgress, false},
		{"Playing", StatusInProgress, false},
		{"in_progress", StatusInProgress, false},
		{"in-progress", StatusInProgress, false},
		{"In Progress", StatusInProgress, false},
		{"inprogress", StatusInProgress, false},
		{"currently playing", StatusInProgress, false},
		{"currentlyplaying", StatusInProgress, false},
		// Planning
		{"want_to_play", StatusPlanning, false},
		{"want-to-play", StatusPlanning, false},
		{"want to play", StatusPlanning, false},
		{"wanttoplay", StatusPlanning, false},
		{"wishlist", StatusPlanning, false},
		{"planning", StatusPlanning, false},
		{"planned", StatusPlanning, false},
		// Paused
		{"paused", StatusPaused, false},
		{"on_hold", StatusPaused, false},
		{"on-hold", StatusPaused, false},
		{"onhold", StatusPaused, false},
		// Dropped
		{"dropped", StatusDropped, false},
		{"abandoned", StatusDropped, false},
		// Unknown
		{"backlog", "", true},
		{"wishihadplayed", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		got, err := FromListName(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("FromListName(%q): error=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("FromListName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
