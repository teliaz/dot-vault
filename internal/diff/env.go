package diff

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"
)

type Env struct {
	Values map[string]string
}

type ChangeType string

const (
	Added     ChangeType = "added"
	Removed   ChangeType = "removed"
	Changed   ChangeType = "changed"
	Unchanged ChangeType = "unchanged"
)

type Change struct {
	Key          string
	Type         ChangeType
	StoredValue  string
	CurrentValue string
}

type Result struct {
	Changes []Change
}

func (r Result) AddedCount() int {
	return r.count(Added)
}

func (r Result) RemovedCount() int {
	return r.count(Removed)
}

func (r Result) ChangedCount() int {
	return r.count(Changed)
}

func (r Result) UnchangedCount() int {
	return r.count(Unchanged)
}

func (r Result) HasDrift() bool {
	return r.AddedCount()+r.RemovedCount()+r.ChangedCount() > 0
}

func (r Result) count(changeType ChangeType) int {
	count := 0
	for _, change := range r.Changes {
		if change.Type == changeType {
			count++
		}
	}
	return count
}

func ParseEnv(payload []byte) (Env, error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Env{}, fmt.Errorf("line %d is not KEY=VALUE", lineNumber)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return Env{}, fmt.Errorf("line %d has an empty key", lineNumber)
		}
		values[key] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return Env{}, fmt.Errorf("scan env: %w", err)
	}

	return Env{Values: values}, nil
}

func Compare(stored Env, current Env) Result {
	keySet := map[string]struct{}{}
	for key := range stored.Values {
		keySet[key] = struct{}{}
	}
	for key := range current.Values {
		keySet[key] = struct{}{}
	}

	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	changes := make([]Change, 0, len(keys))
	for _, key := range keys {
		storedValue, storedExists := stored.Values[key]
		currentValue, currentExists := current.Values[key]

		switch {
		case !storedExists && currentExists:
			changes = append(changes, Change{Key: key, Type: Added, CurrentValue: currentValue})
		case storedExists && !currentExists:
			changes = append(changes, Change{Key: key, Type: Removed, StoredValue: storedValue})
		case storedValue != currentValue:
			changes = append(changes, Change{Key: key, Type: Changed, StoredValue: storedValue, CurrentValue: currentValue})
		default:
			changes = append(changes, Change{Key: key, Type: Unchanged, StoredValue: storedValue, CurrentValue: currentValue})
		}
	}

	return Result{Changes: changes}
}
