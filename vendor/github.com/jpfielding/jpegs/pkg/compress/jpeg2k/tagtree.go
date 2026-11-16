package jpeg2k

// Tag-tree codec for packet headers (ITU-T T.800 B.10.2). Ported from
// dicos.rs crates/jpeg2k/src/tagtree.rs (canonical Taubman / OpenJPEG
// tgt_encode / tgt_decode).
//
// Each interior node holds the minimum of its children. Values are coded
// incrementally: queries at rising thresholds emit or consume only the bits
// needed to refine what the decoder already knows, and per-node state
// persists across queries.

const tagUnknown = ^uint32(0)

type tagNode struct {
	value uint32 // encoder: min-propagated value; decoder: upper bound
	low   uint32 // coding-state lower bound
	known bool   // encoder: a 1 has been emitted here
}

type tagLevel struct {
	w     int
	nodes []tagNode
}

// tagTree covers a leavesW x leavesH grid. levels[0] is the leaf grid; the
// last level is the 1x1 root. A zero dimension yields an empty tree.
type tagTree struct {
	levels           []tagLevel
	leavesW, leavesH int
}

func newTagTree(w, h int) *tagTree {
	t := &tagTree{leavesW: w, leavesH: h}
	if w <= 0 || h <= 0 {
		return t
	}
	for {
		nodes := make([]tagNode, w*h)
		for i := range nodes {
			nodes[i].value = tagUnknown
		}
		t.levels = append(t.levels, tagLevel{w: w, nodes: nodes})
		if w == 1 && h == 1 {
			return t
		}
		w, h = (w+1)/2, (h+1)/2
	}
}

func (t *tagTree) inRange(x, y int) bool {
	return len(t.levels) > 0 && x >= 0 && y >= 0 && x < t.leavesW && y < t.leavesH
}

// set stores a leaf value (encoder side) and propagates the minimum upward.
func (t *tagTree) set(x, y int, v uint32) {
	if !t.inRange(x, y) {
		return
	}
	for l := range t.levels {
		n := &t.levels[l].nodes[y*t.levels[l].w+x]
		n.value = min(n.value, v)
		x, y = x/2, y/2
	}
}

// reset clears coding state but keeps values.
func (t *tagTree) reset() {
	for l := range t.levels {
		for i := range t.levels[l].nodes {
			t.levels[l].nodes[i].low = 0
			t.levels[l].nodes[i].known = false
		}
	}
}

// node returns the node at level l on the path from leaf (x, y).
func (t *tagTree) node(l, x, y int) *tagNode {
	lv := &t.levels[l]
	return &lv.nodes[(y>>l)*lv.w+x>>l]
}

// encode emits, root to leaf, the bits establishing whether leaf (x, y) is
// below threshold.
func (t *tagTree) encode(put func(uint8), x, y int, threshold uint32) {
	if !t.inRange(x, y) {
		return
	}
	var low uint32
	for l := len(t.levels) - 1; l >= 0; l-- {
		n := t.node(l, x, y)
		if low > n.low {
			n.low = low
		} else {
			low = n.low
		}
		for low < threshold {
			if low >= n.value {
				if !n.known {
					put(1)
					n.known = true
				}
				break
			}
			put(0)
			low++
		}
		n.low = low
	}
}

// decode mirrors encode and reports whether leaf (x, y) is established to be
// below threshold. An exhausted bit source returns its error.
func (t *tagTree) decode(next func() (uint8, error), x, y int, threshold uint32) (bool, error) {
	if !t.inRange(x, y) {
		return false, nil
	}
	var low uint32
	for l := len(t.levels) - 1; l >= 0; l-- {
		n := t.node(l, x, y)
		if low > n.low {
			n.low = low
		} else {
			low = n.low
		}
		for low < threshold && low < n.value {
			b, err := next()
			if err != nil {
				return false, err
			}
			if b != 0 {
				n.value = low
			} else {
				low++
			}
		}
		n.low = low
	}
	return t.node(0, x, y).value < threshold, nil
}
