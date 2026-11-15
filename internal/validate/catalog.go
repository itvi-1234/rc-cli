package validate

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
)

type node struct {
	id   Reference
	path string
	def  Definition
}

// catalog holds the extension definitions found beneath a set of directories, by extension id.
type catalog struct{ nodes map[Reference]*node }

var (
	errNotFound  = errors.New("no extension definition")
	errAmbiguous = errors.New("more than one extension definition")
)

// loadCatalog walks roots and indexes every extension definition it finds. A problem with one
// document is recorded and the rest are still read; only a failure to walk a directory is returned.
func loadCatalog(roots []string, issues *report) (*catalog, error) {
	loaded := &catalog{nodes: map[Reference]*node{}}
	seenRoots, seenFiles := map[string]bool{}, map[string]bool{}
	for _, root := range roots {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		if seenRoots[absolute] {
			continue
		}
		seenRoots[absolute] = true
		err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case ".git", "vendor", "node_modules":
					return filepath.SkipDir
				}
				return nil
			}
			if extension := filepath.Ext(path); (extension != ".yaml" && extension != ".yml") || seenFiles[path] {
				return nil
			}
			seenFiles[path] = true
			loaded.add(path, issues)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return loaded, nil
}

func (c *catalog) add(path string, issues *report) {
	definition, isDefinition, err := readDefinition(path)
	if err != nil {
		issues.about(path, "%v", err)
		return
	}
	if !isDefinition {
		return
	}
	id := definition.reference()
	if !id.valid() {
		issues.about(path, "metadata.id is required; metadata.version is optional")
		return
	}
	if existing := c.nodes[id]; existing != nil {
		issues.about(path, "duplicate extension release %s already defined by %s", id, existing.path)
		return
	}
	c.nodes[id] = &node{id: id, path: path, def: definition}
}

// lookup returns the key of the definition a reference names. An exact (id, version) match wins. A
// reference with a version may instead be a short <url>:<version> profile reference. A reference
// without a version names the one definition with that id, whatever its version; two or more is ambiguous.
func (c *catalog) lookup(reference Reference) (Reference, error) {
	if c.nodes[reference] != nil {
		return reference, nil
	}
	if reference.Version != "" {
		if expanded := expandShort(reference); c.nodes[expanded] != nil {
			return expanded, nil
		}
		return reference, fmt.Errorf("%w for %s", errNotFound, reference)
	}
	var matches []Reference
	for key := range c.nodes {
		if key.ID == reference.ID {
			matches = append(matches, key)
		}
	}
	switch len(matches) {
	case 0:
		return reference, fmt.Errorf("%w for %s", errNotFound, reference)
	case 1:
		return matches[0], nil
	}
	slices.SortFunc(matches, Reference.compare)
	return reference, fmt.Errorf("%w with id %s: %v", errAmbiguous, reference.ID, matches)
}

// resolve returns the vocabulary of id with its transitive dependencies, dependencies first.
func (c *catalog) resolve(id Reference) vocabulary {
	seen := map[Reference]bool{}
	var nodes []*node
	var visit func(Reference)
	visit = func(current Reference) {
		if seen[current] {
			return
		}
		if resolved, err := c.lookup(current); err == nil {
			current = resolved
		}
		if seen[current] {
			return
		}
		seen[current] = true
		found := c.nodes[current]
		if found == nil {
			return
		}
		for _, dependency := range found.def.Spec.Dependencies {
			visit(dependency)
		}
		nodes = append(nodes, found)
	}
	visit(id)
	return vocabulary{nodes: nodes}
}
