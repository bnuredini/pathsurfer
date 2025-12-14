package fuzzy

import (
	"unicode"
	"unicode/utf8"
)

type Match struct {
	CandidateString string
	Indexes         []int
	Score           int
}

const (
	FirstCharBonus   = 5
	SeparatorBonus   = 10
	CamelCaseBonus   = 8
	ConsecutiveBonus = 5
)

func Find(rawPattern string, candidates []string) []Match {
	if len(rawPattern) == 0 || len(candidates) == 0 {
		return []Match{}
	}

	result := []Match{}
	pattern := []rune(rawPattern)

	patternIdx := 0
	runningBestScore := -1
	runningMatchIdx := -1

	for _, candidate := range candidates {
		match := Match{CandidateString: candidate}

		var candidatePrev rune
		var candidatePrevSize int
		candidateCurr, candidateCurrSize := utf8.DecodeRuneInString(candidate)

		for candidateIdx := 0; candidateIdx < len(candidate); candidateIdx += candidatePrevSize {
			if equalsIgnoreCase(pattern[patternIdx], candidateCurr) {
				score := 1

				if candidateIdx == 0 {
					score += FirstCharBonus
				}

				if candidateIdx != 0 && (candidateIdx-1 == runningMatchIdx) {
					score += ConsecutiveBonus
				}

				if isSeparator(candidatePrev) {
					score += SeparatorBonus
				}

				if unicode.IsLower(candidatePrev) && unicode.IsUpper(candidateCurr) {
					score += CamelCaseBonus
				}

				if score > runningBestScore {
					runningBestScore = score
					runningMatchIdx = candidateIdx
				}
			}

			var candidateNext rune
			var candidateNextSize int
			candidateNextIdx := candidateIdx + candidateCurrSize

			if candidateNextIdx < len(candidate) {
				candidateNext, candidateNextSize = utf8.DecodeRuneInString(candidate[candidateNextIdx:])
			}

			var patternNext rune
			if patternIdx+1 < len(pattern) {
				patternNext = pattern[patternIdx+1]
			}

			if equalsIgnoreCase(patternNext, candidateNext) || candidateNext == 0 {
				patternIdx++

				if runningMatchIdx == -1 {
					break
				}

				match.Score += runningBestScore
				match.Indexes = append(match.Indexes, runningMatchIdx)

				runningBestScore = -1
				runningMatchIdx = -1
			}

			candidatePrev = candidateCurr
			candidatePrevSize = candidateCurrSize

			candidateCurr = candidateNext
			candidateCurrSize = candidateNextSize
		}

		patternIdx = 0

		if len(pattern) != len(match.Indexes) {
			continue
		}

		distPenalty := 0
		for i := 1; i < len(match.Indexes); i++ {
			distPenalty += match.Indexes[i] - match.Indexes[i-1]
		}
		match.Score -= distPenalty

		result = append(result, match)
	}

	return result
}

func equalsIgnoreCase(searchChar, targetChar rune) bool {
	if searchChar == targetChar {
		return true
	}

	if searchChar < targetChar {
		searchChar, targetChar = targetChar, searchChar
	}

	if searchChar < utf8.RuneSelf {
		if 'A' <= targetChar && targetChar <= 'Z' && searchChar == targetChar+'a'-'A' {
			return true
		}
		return false
	}

	r := unicode.SimpleFold(targetChar)
	for r != targetChar && r < searchChar {
		r = unicode.SimpleFold(r)
	}

	return r == searchChar
}

func isSeparator(r rune) bool {
	return r == '_' || r == '-' || r == '/'
}
