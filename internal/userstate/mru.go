package userstate

// PushMRU inserts entry at the front of list, removes prior equal entry (byte-wise), and trims to max.
// Empty entry leaves list unchanged.
func PushMRU(list []string, entry string, max int) []string {
	if entry == "" || max <= 0 {
		return list
	}
	out := make([]string, 0, len(list)+1)
	out = append(out, entry)
	for _, s := range list {
		if s == entry {
			continue
		}
		out = append(out, s)
		if len(out) >= max {
			break
		}
	}
	if len(out) > max {
		out = out[:max]
	}
	return out
}
