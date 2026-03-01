package highlight

import (
	"log"
	"regexp"
	"strings"

	"github.com/micro-editor/micro/v2/internal/util"
)

// A State represents the region at the end of a line
type State *region

// LineStates is an interface for a buffer-like object which can also store the states and matches for every line
type LineStates interface {
	LineBytes(n int) []byte
	LinesNum() int
	State(lineN int) State
	SetState(lineN int, s State)
	SetMatch(lineN int, m LineMatch)
	Lock()
	Unlock()
}

// branch is used to build branches of regions and patterns per line
type branch struct {
	start    int
	end      int
	closed   bool
	group    Group
	region   *region
	rules    *rules
	parent   *branch
	branches []*branch
}

// A Highlighter contains the information needed to highlight a string
type Highlighter struct {
	Def            *Def
	root           branch
	fullHighlights []Group
}

// NewHighlighter returns a new highlighter from the given syntax definition
func NewHighlighter(def *Def) *Highlighter {
	h := new(Highlighter)
	h.Def = def
	return h
}

// LineMatch represents the syntax highlighting matches for one line. Each index where the coloring is changed is marked with that
// color's group (represented as one byte)
type LineMatch map[int]Group

func findIndex(regex *regexp.Regexp, skip *regexp.Regexp, str []byte) []int {
	var strbytes []byte
	if skip != nil {
		strbytes = skip.ReplaceAllFunc(str, func(match []byte) []byte {
			res := make([]byte, util.CharacterCount(match))
			return res
		})
	} else {
		strbytes = str
	}

	match := regex.FindIndex(strbytes)
	if match == nil {
		return nil
	}
	return []int{util.RunePos(str, match[0]), util.RunePos(str, match[1])}
}

func findAllIndex(regex *regexp.Regexp, skip *regexp.Regexp, str []byte) [][]int {
	var strbytes []byte
	if skip != nil {
		strbytes = skip.ReplaceAllFunc(str, func(match []byte) []byte {
			res := make([]byte, util.CharacterCount(match))
			return res
		})
	} else {
		strbytes = str
	}

	matches := regex.FindAllIndex(strbytes, -1)
	for i, m := range matches {
		matches[i][0] = util.RunePos(str, m[0])
		matches[i][1] = util.RunePos(str, m[1])
	}
	return matches
}

func (b *branch) process(newBranch *branch) *branch {
	i, j := 0, 0
	for k, current := range b.branches {
		// Do not insert if newBranch starts inside an existing branch,
		// but maintain backward compatibility with existing definitions
		// in which later rules have a higher priority, thus allow equal starts.
		if current.start < newBranch.start && newBranch.start < current.end {
			return nil
		}
		// Keep valid branches in front.
		if current.end <= newBranch.start {
			i++
			j = i
			continue
		}
		// Find the last branch covered by newBranch.
		if current.start < newBranch.end {
			j = k + 1
		}
	}

	log.Println("newBranch.start:", newBranch.start, "newBranch.end:", newBranch.end)

	// Replace branches[i:j] with newBranch.
	newBranches := make([]*branch, 0, len(b.branches)-(j-i)+1)
	newBranches = append(newBranches, b.branches[:i]...)
	newBranches = append(newBranches, newBranch)
	newBranches = append(newBranches, b.branches[j:]...)
	b.branches = newBranches

	return newBranch
}

func (h *Highlighter) highlightPatterns(start int, line []byte, curRegion *region, b *branch) {
	lineLen := util.CharacterCount(line)
	log.Println("highlightPatterns: start:", start, "line:", string(line))
	if lineLen == 0 {
		return
	}

	for _, p := range b.rules.patterns {
		log.Println("p.regex:", p.regex.String())
		matches := findAllIndex(p.regex, nil, line)
		for _, m := range matches {
			b.process(&branch{start + m[0], start + m[1], true, p.group, nil, nil, b, []*branch{}})
		}
	}
}

func (h *Highlighter) highlightRegions(start int, line []byte, curRegion *region, b *branch) {
	lineLen := util.CharacterCount(line)
	log.Println("highlightRegions: start:", start, "line:", string(line))
	if lineLen == 0 {
		return
	}

	h.highlightPatterns(start, line, curRegion, b)

regionLoop:
	for _, r := range b.rules.regions {
		log.Println("r.start:", r.start.String(), "r.end:", r.end.String())
		if curRegion != nil && curRegion != r {
			continue
		}
		startMatches := findAllIndex(r.start, r.skip, line)
		endMatches := findAllIndex(r.end, r.skip, line)
		samePattern := r.start.String() == r.end.String()
		lenStartMatches := len(startMatches)
		lenEndMatches := len(endMatches)
	startLoop:
		for startIdx := 0; startIdx < lenStartMatches; startIdx++ {
			log.Println("startIdx:", startIdx+1, "of", lenStartMatches)
			startMatch := startMatches[startIdx]
			for endIdx := 0; endIdx < lenEndMatches; endIdx++ {
				log.Println("startIdx:", startIdx+1, "of", lenStartMatches, "/ endIdx:", endIdx+1, "of", lenEndMatches)
				endMatch := endMatches[endIdx]
				if startMatch[0] == endMatch[0] {
					if samePattern {
						// start and end are the same
						log.Println("start == end")
						if curRegion == r {
							continue startLoop
						} else {
							continue
						}
					}
				} else if startMatch[1] <= endMatch[0] {
					// start and end at the current line
					if samePattern {
						startIdx += 1
					}
					c := b.process(&branch{start + startMatch[0], start + endMatch[1], true, r.group, r, r.rules, b, []*branch{}})
					if c != nil {
						log.Println("start < end")
						c.process(&branch{start + startMatch[0], start + startMatch[1], true, r.limitGroup, nil, nil, c, []*branch{}})
						mid := c.process(&branch{start + startMatch[1], start + endMatch[0], true, r.group, r, r.rules, c, []*branch{}})
						c.process(&branch{start + endMatch[0], start + endMatch[1], true, r.limitGroup, nil, nil, c, []*branch{}})
						h.highlightRegions(start+startMatch[1], util.SliceStartEnd(line, startMatch[1], endMatch[0]), nil, mid)
					}
					continue startLoop
				} else if endMatch[1] <= startMatch[0] && curRegion == r {
					// end at the current line and next start found
					c := b.process(&branch{start, start + endMatch[1], true, r.group, r, r.rules, b, []*branch{}})
					if c != nil {
						log.Println("... end")
						curRegion = curRegion.parent
						mid := c.process(&branch{start, start + endMatch[0], true, r.group, r, r.rules, c, []*branch{}})
						c.process(&branch{start + endMatch[0], start + endMatch[1], true, r.limitGroup, nil, nil, c, []*branch{}})
						h.highlightRegions(start, util.SliceStart(line, endMatch[0]), nil, mid)
						// if curRegion.parent != nil {
						// 	h.highlightRegions(start+endMatch[1], util.SliceEnd(line, endMatch[1]), curRegion.parent, c.parent)
						// }
					}
					continue
				}
			}
			if lenEndMatches == 0 || (samePattern && lenStartMatches == lenEndMatches) {
				// start at the current line
				c := b.process(&branch{start + startMatch[0], start + lineLen, false, r.group, r, r.rules, b, []*branch{}})
				if c != nil {
					log.Println("start ...")
					c.process(&branch{start + startMatch[0], start + startMatch[1], false, r.limitGroup, nil, nil, c, []*branch{}})
					mid := c.process(&branch{start + startMatch[1], start + lineLen, false, r.group, r, r.rules, c, []*branch{}})
					h.highlightRegions(start+startMatch[1], util.SliceEnd(line, startMatch[1]), nil, mid)
				}
				continue regionLoop
			}
		}
		if curRegion == r && (lenStartMatches == 0 || (samePattern && lenStartMatches == lenEndMatches)) {
			for _, endMatch := range endMatches {
				// end at the current line
				c := b.process(&branch{start, start + endMatch[1], true, r.group, r, r.rules, b, []*branch{}})
				if c != nil {
					log.Println("... end")
					curRegion = curRegion.parent
					mid := c.process(&branch{start, start + endMatch[0], true, r.group, r, r.rules, c, []*branch{}})
					c.process(&branch{start + endMatch[0], start + endMatch[1], true, r.limitGroup, nil, nil, c, []*branch{}})
					h.highlightRegions(start, util.SliceStart(line, endMatch[0]), nil, mid)
					// if curRegion.parent != nil {
					// 	h.highlightRegions(start+endMatch[1], util.SliceEnd(line, endMatch[1]), curRegion.parent, c.parent)
					// }
				}
			}
		}
	}
	// if curRegion != nil && len(b.rules.regions) == 0 {
	if curRegion != nil {
		// current region still open
		c := b.process(&branch{start, lineLen, false, curRegion.group, curRegion, curRegion.rules, b, []*branch{}})
		if c != nil {
			log.Println("...")
			h.highlightRegions(start, line, nil, c)
		}
	}
}

func (b *branch) parentsOpen() bool {
	if b.parent != nil {
		if b.parent.closed {
			return false
		}
		return b.parent.parentsOpen()
	}
	return true
}

func (h *Highlighter) flatten(branches []*branch) *region {
	var lastOpen *region
	for _, branch := range branches {
		for i := branch.start; i < branch.end; i++ {
			h.fullHighlights[i] = branch.group
		}
		if !branch.closed && branch.parentsOpen() {
			lastOpen = branch.region
		}
		tmpLastOpen := h.flatten(branch.branches)
		if tmpLastOpen != nil {
			lastOpen = tmpLastOpen
		}
	}
	return lastOpen
}

func (h *Highlighter) highlight(highlights LineMatch, start int, lineNum int, line []byte, curRegion *region) (LineMatch, *region) {
	lineLen := util.CharacterCount(line)
	log.Println("highlight: lineNum:", lineNum, "start:", start, "line:", string(line))
	if lineLen == 0 {
		return highlights, curRegion
	}

	h.root = branch{}
	if curRegion != nil && curRegion.parent != nil {
		h.root.rules = curRegion.parent.rules
	} else {
		h.root.rules = h.Def.rules
	}

	h.highlightRegions(start, line, curRegion, &h.root)

	h.fullHighlights = make([]Group, lineLen)
	lastRegion := h.flatten(h.root.branches)

	for i, g := range h.fullHighlights {
		if i == 0 || g != h.fullHighlights[i-1] {
			highlights[i] = g
		}
	}

	return highlights, lastRegion
}

// HighlightString syntax highlights a string
// Use this function for simple syntax highlighting and use the other functions for
// more advanced syntax highlighting. They are optimized for quick rehighlighting of the same
// text with minor changes made
func (h *Highlighter) HighlightString(input string) []LineMatch {
	lines := strings.Split(input, "\n")
	var lineMatches []LineMatch
	var curState *region

	for i := 0; i < len(lines); i++ {
		line := []byte(lines[i])
		highlights := make(LineMatch)
		var match LineMatch
		match, curState = h.highlight(highlights, 0, i, line, curState)
		lineMatches = append(lineMatches, match)
	}

	return lineMatches
}

// Highlight sets the state and matches for each line from startline to endline,
// and also for some amount of lines after endline, until it detects a line
// whose state does not change, which means that the lines after it do not change
// their highlighting and therefore do not need to be updated.
func (h *Highlighter) Highlight(input LineStates, startline, endline int) {
	var curState *region
	if startline > 0 {
		input.Lock()
		if startline-1 < input.LinesNum() {
			curState = input.State(startline - 1)
		}
		input.Unlock()
	}

	for i := startline; ; i++ {
		input.Lock()
		if i >= input.LinesNum() {
			input.Unlock()
			break
		}

		line := input.LineBytes(i)
		highlights := make(LineMatch)

		var match LineMatch
		match, curState = h.highlight(highlights, 0, i, line, curState)

		var lastState *region
		if i >= endline {
			lastState = input.State(i)
		}

		input.SetState(i, curState)
		input.SetMatch(i, match)
		input.Unlock()

		if i >= endline && curState == lastState {
			break
		}
	}
}

// ReHighlightLine will rehighlight the state and match for a single line
func (h *Highlighter) ReHighlightLine(input LineStates, lineN int) {
	input.Lock()
	defer input.Unlock()

	line := input.LineBytes(lineN)
	highlights := make(LineMatch)

	var curState *region
	if lineN > 0 {
		curState = input.State(lineN - 1)
	}

	var match LineMatch
	match, curState = h.highlight(highlights, 0, lineN, line, curState)

	input.SetState(lineN, curState)
	input.SetMatch(lineN, match)
}
