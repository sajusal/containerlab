package clab

import (
	"fmt"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

type dependencyManager struct {
	// map of WaitGroup items per node.
	// the scheduling of the nodes creation is dependent on this WaitGroup.
	// other nodes, that the specific node relies on will increment the WaitGroup by one.
	nodeWaitGroup map[string]*sync.WaitGroup
	// To keep book about which nodes depend on node x, the waitgroups of the dependent Nodes are listed here.
	// on successful creation of node x, all the dependent nodes Waitgroups will be decremented.
	nodeWaiter map[string][]string
}

func NewDependencyManager() *dependencyManager {
	return &dependencyManager{
		nodeWaitGroup: map[string]*sync.WaitGroup{},
		nodeWaiter:    map[string][]string{},
	}
}

func (dm *dependencyManager) AddNodeEntry(name string) {
	// contains the waitgroup per node
	dm.nodeWaitGroup[name] = &sync.WaitGroup{}
	// contains the references to the wait groups that wait for the named node
	// all these will have to be decreased on finishing the startup of the named node
	dm.nodeWaiter[name] = []string{}
}

// AddDependency adds a dependency between dependentNodeName and dependingNodeName.
// the dependingNode will wait for the dependentNode to become available.
func (dm *dependencyManager) AddDependency(dependentNodeName, dependingNodeName string) {
	// increase it by one
	dm.nodeWaitGroup[dependingNodeName].Add(1)
	// add it to the static node waiter, such that on finishing these are decreased by 1
	dm.nodeWaiter[dependentNodeName] = append(dm.nodeWaiter[dependentNodeName], dependingNodeName)
}

// WaitForDependenciesToFinishFor is called by a node that is meant to be created.
// this call will bock until all the defined dependencies are (other containers) are created before
// the call returns.
func (dm *dependencyManager) WaitForDependenciesToFinishFor(nodeName string) {
	dm.nodeWaitGroup[nodeName].Wait()
}

// SignalDone is called by a node that has finished the creation process.
// internally the dependent nodes will be "notified" that an additional (if multiple exist) dependency is satisfied.
func (dm *dependencyManager) SignalDone(nodeName string) {
	for _, waiterNodeName := range dm.nodeWaiter[nodeName] {
		dm.nodeWaitGroup[waiterNodeName].Done()
	}
}

// CheckAcyclicity checks the dependencies between the defined namespaces and throws an error if.
func (dm *dependencyManager) CheckAcyclicity() error {
	log.Debugf("Dependencies:\n%s", dm.String())
	if !isAcyclic(dm.nodeWaiter, 1) {
		return fmt.Errorf("the dependencies defned on the namespaces are not resolvable.\n%s", dm.String())
	}

	return nil
}

// isAcyclic checks the provided data for cycles.
// i is just for visual candy in the debug output. Must be set to 1.
func isAcyclic(dependencies map[string][]string, i int) bool {
	// debug output
	d := []string{}
	for name, entries := range dependencies {
		d = append(d, fmt.Sprintf("%s <- [ %s ]", name, strings.Join(entries, ", ")))
	}
	log.Debugf("- cyclicity check round %d - \n%s", i, strings.Join(d, "\n"))

	// no more nodes then the graph is acyclic
	if len(dependencies) == 0 {
		log.Debugf("node creation graph is successfully validated as being acyclic")
		return true
	}

	remainingDeps := map[string][]string{}
	leafNodes := []string{}
	// mark a node as a remaining dependency if other nodes still depend on it,
	// otherwise add it to the leaf list for it to be removed in the next round of recursive check
	for name, deps := range dependencies {
		if len(deps) > 0 {
			remainingDeps[name] = deps
		} else {
			leafNodes = append(leafNodes, name)
		}
	}

	// if nodes remain but none of them is a leaf node, must by cyclic
	if len(leafNodes) == 0 {
		return false
	}

	// iterate over remaining nodes, to remove all leaf nodes from the dependencies, because in the next round of recursion,
	// these will no longer be there, they suffice the satisfy the acyclicity property
	for name, deps := range remainingDeps {
		// new array that keeps track of remaining dependencies
		remainingNodeDeps := []string{}
		// iterate over deleted nodes
		for _, dep := range deps {
			keep := true
			// check if the actual dep is a leafNode and should therefore be removed
			for _, delnode := range leafNodes {
				// if it is a node that is meant to be deleted, stop here and make sure its not taken over to the new array
				if delnode == dep {
					keep = false
					break
				}
			}
			if keep {
				remainingNodeDeps = append(remainingNodeDeps, dep)
			}
		}
		// replace previous with the new, cleanup dependencies.
		remainingDeps[name] = remainingNodeDeps
	}
	return isAcyclic(remainingDeps, i+1)
}

// String returns a string representation of the actual dependencies.
func (dm *dependencyManager) String() string {
	// map to record the dependencies in string based representation
	dependencies := map[string][]string{}

	// prepare lookup table
	for name := range dm.nodeWaitGroup {
		// populate dependency map already with empty arrays
		dependencies[name] = []string{}
	}

	// build the dependency datastruct
	for name, wgarray := range dm.nodeWaiter {
		for _, waiter := range wgarray {
			dependencies[waiter] = append(dependencies[waiter], name)
		}
	}

	result := []string{}
	// print dependencies
	for nodename, deps := range dependencies {
		result = append(result, fmt.Sprintf("%s -> [ %s ]", nodename, strings.Join(deps, ", ")))
	}
	return strings.Join(result, "\n")
}
