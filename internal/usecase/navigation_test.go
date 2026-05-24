package usecase

import "testing"

func TestNavigationSingleLineInteriorMovementKeepsScrollOffsetStable(t *testing.T) {
	state, err := NewNavigationState(NavigationOptions{
		OutputCount:       100,
		ViewportHeight:    5,
		CursorOutputIndex: 2,
		ScrollOffset:      0,
	})
	if err != nil {
		t.Fatal(err)
	}

	state.Move(NavigationMoveDown)

	assertNavigationState(t, state, 3, 0, false)
}

func TestNavigationSingleLineBoundaryMovementScrollsExactlyOneLine(t *testing.T) {
	tests := []struct {
		name       string
		options    NavigationOptions
		move       NavigationMove
		wantCursor int
		wantScroll int
	}{
		{
			name: "bottom boundary moving down",
			options: NavigationOptions{
				OutputCount:       100,
				ViewportHeight:    5,
				CursorOutputIndex: 4,
				ScrollOffset:      0,
			},
			move:       NavigationMoveDown,
			wantCursor: 5,
			wantScroll: 1,
		},
		{
			name: "top boundary moving up",
			options: NavigationOptions{
				OutputCount:       100,
				ViewportHeight:    5,
				CursorOutputIndex: 10,
				ScrollOffset:      10,
			},
			move:       NavigationMoveUp,
			wantCursor: 9,
			wantScroll: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := NewNavigationState(tt.options)
			if err != nil {
				t.Fatal(err)
			}

			state.Move(tt.move)

			assertNavigationState(t, state, tt.wantCursor, tt.wantScroll, false)
		})
	}
}

func TestNavigationPageMovementUsesTwoStepEdgeFirstPolicy(t *testing.T) {
	t.Run("PageDown moves to bottom edge before scrolling", func(t *testing.T) {
		state := mustNavigationState(t, NavigationOptions{
			OutputCount:       100,
			ViewportHeight:    5,
			CursorOutputIndex: 2,
			ScrollOffset:      0,
		})

		state.Move(NavigationMovePageDown)
		assertNavigationState(t, state, 4, 0, false)

		state.Move(NavigationMovePageDown)
		assertNavigationState(t, state, 9, 5, false)
	})

	t.Run("PageUp moves to top edge before scrolling", func(t *testing.T) {
		state := mustNavigationState(t, NavigationOptions{
			OutputCount:       100,
			ViewportHeight:    5,
			CursorOutputIndex: 12,
			ScrollOffset:      10,
		})

		state.Move(NavigationMovePageUp)
		assertNavigationState(t, state, 10, 10, false)

		state.Move(NavigationMovePageUp)
		assertNavigationState(t, state, 5, 5, false)
	})
}

func TestNavigationHomeEndAndGPositionFirstAndLastOutputLogs(t *testing.T) {
	t.Run("Home moves to first output log", func(t *testing.T) {
		state := mustNavigationState(t, NavigationOptions{
			OutputCount:       20,
			ViewportHeight:    5,
			CursorOutputIndex: 10,
			ScrollOffset:      8,
			Follow:            true,
		})

		state.Move(NavigationMoveHome)

		assertNavigationState(t, state, 0, 0, false)
	})

	t.Run("End moves to last output log with last output at screen bottom", func(t *testing.T) {
		state := mustNavigationState(t, NavigationOptions{
			OutputCount:       20,
			ViewportHeight:    5,
			CursorOutputIndex: 3,
			ScrollOffset:      0,
		})

		state.Move(NavigationMoveEnd)

		assertNavigationState(t, state, 19, 15, true)
	})

	t.Run("G moves to last output log and enables follow", func(t *testing.T) {
		state := mustNavigationState(t, NavigationOptions{
			OutputCount:       20,
			ViewportHeight:    5,
			CursorOutputIndex: 3,
			ScrollOffset:      0,
		})

		state.Move(NavigationMoveLastAndFollow)

		assertNavigationState(t, state, 19, 15, true)
	})
}

func TestNavigationFollowModeTransitionsAndPinsTail(t *testing.T) {
	t.Run("moving to last output enables follow", func(t *testing.T) {
		state := mustNavigationState(t, NavigationOptions{
			OutputCount:       6,
			ViewportHeight:    5,
			CursorOutputIndex: 4,
			ScrollOffset:      1,
		})

		state.Move(NavigationMoveDown)

		assertNavigationState(t, state, 5, 1, true)
	})

	t.Run("moving away from last output disables follow", func(t *testing.T) {
		state := mustNavigationState(t, NavigationOptions{
			OutputCount:       6,
			ViewportHeight:    5,
			CursorOutputIndex: 5,
			ScrollOffset:      1,
			Follow:            true,
		})

		state.Move(NavigationMoveUp)

		assertNavigationState(t, state, 4, 1, false)
	})

	t.Run("output growth while following pins cursor and last output to screen bottom", func(t *testing.T) {
		state := mustNavigationState(t, NavigationOptions{
			OutputCount:       6,
			ViewportHeight:    5,
			CursorOutputIndex: 5,
			ScrollOffset:      1,
			Follow:            true,
		})

		state.SetOutputCount(8)

		assertNavigationState(t, state, 7, 3, true)
	})

	t.Run("output growth while not following does not auto scroll", func(t *testing.T) {
		state := mustNavigationState(t, NavigationOptions{
			OutputCount:       6,
			ViewportHeight:    5,
			CursorOutputIndex: 2,
			ScrollOffset:      0,
		})

		state.SetOutputCount(8)

		assertNavigationState(t, state, 2, 0, false)
	})
}

func mustNavigationState(t *testing.T, options NavigationOptions) *NavigationState {
	t.Helper()

	state, err := NewNavigationState(options)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func assertNavigationState(t *testing.T, state *NavigationState, wantCursor, wantScroll int, wantFollow bool) {
	t.Helper()

	if got := state.CursorOutputIndex(); got != wantCursor {
		t.Fatalf("cursor output index = %d, want %d", got, wantCursor)
	}
	if got := state.ScrollOffset(); got != wantScroll {
		t.Fatalf("scroll offset = %d, want %d", got, wantScroll)
	}
	if got := state.Follow(); got != wantFollow {
		t.Fatalf("follow = %v, want %v", got, wantFollow)
	}
}
