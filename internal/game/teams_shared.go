//gojs:shared

package game

// The team join limit (mp_limitteams), shared with the client (shared.gen.js):
// the server refuses a join over it (Server.teamFull) and the team menu shows
// the same side full (web/assets/teams.js teamOpen). Vectors both sides
// check: testdata/teamlimit_cases.json.

// TeamOverLimit: would a side of joined players (not counting the joiner) be
// more than limit players bigger than a side of other after one more joins?
// A limit of 0 or less is no limit (CS: mp_limitteams 0).
func TeamOverLimit(joined, other, limit int) bool {
	return limit > 0 && joined+1-other > limit
}
