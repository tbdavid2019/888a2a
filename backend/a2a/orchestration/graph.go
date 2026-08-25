package orchestration

import (
	"context"
	"sync"

	"github.com/pkg/errors"

	"github.com/Ranxy/laelia/backend/a2a"
	"github.com/Ranxy/laelia/backend/manager/store"
)

var (
	// ErrDirectCycle indicates a task attempts to set itself as parent.
	ErrDirectCycle = errors.Wrap(a2a.ErrCyclicDelegation, "direct cycle: parent work ID cannot equal child work ID")
	// ErrIndirectCycle indicates a cyclic dependency exists in the delegation graph.
	ErrIndirectCycle = errors.Wrap(a2a.ErrCyclicDelegation, "indirect cycle detected in active delegation graph")
)

// CycleDetector checks for cycles before committing a delegation edge.
type CycleDetector struct {
	store *store.Store
}

// NewCycleDetector creates a new cycle detector backed by the work store.
func NewCycleDetector(store *store.Store) *CycleDetector {
	return &CycleDetector{store: store}
}

// CheckProposedEdge validates that adding an edge from parentWorkID to proposedChildWorkID does not form a cycle.
func (c *CycleDetector) CheckProposedEdge(ctx context.Context, tenantID, parentWorkID, proposedChildWorkID string) error {
	if parentWorkID == "" || proposedChildWorkID == "" {
		return nil
	}

	// 1. Direct cycle: A -> A
	if parentWorkID == proposedChildWorkID {
		return ErrDirectCycle
	}

	if c.store == nil {
		return nil
	}
	if tenantID == "" {
		tenantID = "default"
	}

	// 2. Indirect cycle: traverse parent chain upwards starting from parentWorkID.
	// If proposedChildWorkID is encountered as an ancestor of parentWorkID, adding child would form a cycle.
	currentID := parentWorkID
	visited := map[string]struct{}{parentWorkID: {}}

	for i := 0; i < 100; i++ {
		parentWork, err := c.store.GetWork(ctx, tenantID, currentID)
		if err != nil {
			if errors.Is(err, store.ErrWorkNotFound) {
				break
			}
			return errors.Wrapf(err, "inspect ancestor work %s", currentID)
		}

		if !parentWork.ParentWorkID.Valid || parentWork.ParentWorkID.String == "" {
			break
		}

		ancestorID := parentWork.ParentWorkID.String
		if ancestorID == proposedChildWorkID {
			return errors.Wrapf(ErrIndirectCycle, "proposed child %s is already an ancestor of parent %s", proposedChildWorkID, parentWorkID)
		}

		if _, seen := visited[ancestorID]; seen {
			return errors.Wrapf(ErrIndirectCycle, "existing cycle detected at %s", ancestorID)
		}
		visited[ancestorID] = struct{}{}
		currentID = ancestorID
	}

	return nil
}

// TaskGraph represents an in-memory directed acyclic graph of tasks.
type TaskGraph struct {
	mu        sync.RWMutex
	nodes     map[string]*TaskNode
	edges     map[string][]string // parent -> children
	backEdges map[string]string   // child -> parent
}

// TaskNode represents a node in the TaskGraph.
type TaskNode struct {
	WorkID   string
	ParentID string
	State    string
	Depth    int32
}

// NewTaskGraph initializes an empty TaskGraph.
func NewTaskGraph() *TaskGraph {
	return &TaskGraph{
		nodes:     make(map[string]*TaskNode),
		edges:     make(map[string][]string),
		backEdges: make(map[string]string),
	}
}

// AddNode adds a task node to the graph.
func (g *TaskGraph) AddNode(node *TaskNode) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.nodes[node.WorkID] = node
	if node.ParentID != "" {
		g.edges[node.ParentID] = append(g.edges[node.ParentID], node.WorkID)
		g.backEdges[node.WorkID] = node.ParentID
	}
}

// CheckCycle verifies if adding parentID -> childID would create a cycle.
func (g *TaskGraph) CheckCycle(parentID, childID string) error {
	if parentID == childID {
		return ErrDirectCycle
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	// Walk backEdges from parentID to root
	curr := parentID
	visited := map[string]struct{}{parentID: {}}

	for {
		p, exists := g.backEdges[curr]
		if !exists || p == "" {
			break
		}
		if p == childID {
			return errors.Wrapf(ErrIndirectCycle, "child %s is ancestor of %s", childID, parentID)
		}
		if _, seen := visited[p]; seen {
			return errors.Wrapf(ErrIndirectCycle, "cycle detected at %s", p)
		}
		visited[p] = struct{}{}
		curr = p
	}

	return nil
}

// GetDescendants returns all descendant work IDs for a given root.
func (g *TaskGraph) GetDescendants(rootID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var descendants []string
	queue := []string{rootID}
	visited := map[string]struct{}{rootID: {}}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		children := g.edges[curr]
		for _, child := range children {
			if _, seen := visited[child]; !seen {
				visited[child] = struct{}{}
				descendants = append(descendants, child)
				queue = append(queue, child)
			}
		}
	}

	return descendants
}
