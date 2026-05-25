package usecase

const MaxInputHistoryEntries = 10

// InputChannelHistory holds the most recently applied value and prior entries for one input.
type InputChannelHistory struct {
	Last    string
	History []string
}

// InputHistorySnapshot is the in-memory shape of persisted filter/search history.
type InputHistorySnapshot struct {
	Filter InputChannelHistory
	Search InputChannelHistory
}

// RecordHistoryEntry inserts value at the front, removes duplicates, and caps length.
func RecordHistoryEntry(history []string, value string) []string {
	if value == "" {
		return history
	}
	updated := []string{value}
	for _, entry := range history {
		if entry != value {
			updated = append(updated, entry)
		}
	}
	if len(updated) > MaxInputHistoryEntries {
		updated = updated[:MaxInputHistoryEntries]
	}
	return updated
}

// RecordChannelHistory updates last and history for one channel after a successful apply.
func RecordChannelHistory(channel InputChannelHistory, value string) InputChannelHistory {
	if value == "" {
		channel.Last = ""
		return channel
	}
	channel.Last = value
	channel.History = RecordHistoryEntry(channel.History, value)
	return channel
}
