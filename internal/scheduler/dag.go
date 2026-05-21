package scheduler

import "fmt"

func validateDAG(taskIDs []string, deps map[string][]string) error {
	inDegree := make(map[string]int)
	for _, id := range taskIDs {
		inDegree[id] = 0
	}
	for _, id := range taskIDs {
		for _, dep := range deps[id] {
			if _, ok := inDegree[dep]; !ok {
				return fmt.Errorf("task %s depends on unknown task %s", id, dep)
			}
			inDegree[id]++
		}
	}

	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visited := 0
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		visited++

		for _, id := range taskIDs {
			for _, dep := range deps[id] {
				if dep == curr {
					inDegree[id]--
					if inDegree[id] == 0 {
						queue = append(queue, id)
					}
				}
			}
		}
	}

	if visited != len(taskIDs) {
		return fmt.Errorf("circular dependency detected among tasks")
	}
	return nil
}
