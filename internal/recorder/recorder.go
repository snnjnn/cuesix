package recorder

import (
	"encoding/json"
	"log/slog"

	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/cursor"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
	"go.yaml.in/yaml/v4"
)

// Descriptor holds metadata of a config object
type Descriptor struct {
	Tags  map[string][]string `json:"tags"`  // Tags associated to the object
	Paths []string            `json:"paths"` // Source keys that contribute to this object
}

// Records sources and generated configs
type Recorder struct {
	// Public parameters
	Logger           *slog.Logger
	Enumerator       *SourcesEnumerator
	ValidatorFactory dispatcher.ValidatorFactory
	// Instances
	instances map[string]*instance
}

type instance struct {
	cursor.Lock
	logger     *slog.Logger
	virtualgw  string
	validator  dispatcher.Validator
	enumerator *SourcesEnumerator
	candidate  []byte
	isYaml     bool
	// Index of sources by kind and ID
	snippets map[string]compiler.Snippet
	index    map[string]map[string]Descriptor
}

// NewRecorder creates a recorder for sources, validation, and committed output.
func NewRecorder(logger *slog.Logger, sources *SourcesEnumerator, validatorFactory dispatcher.ValidatorFactory) *Recorder {
	return &Recorder{
		Logger:           logger,
		Enumerator:       sources,
		ValidatorFactory: validatorFactory,
		instances:        make(map[string]*instance),
	}
}

// Instance returns a validator wrapper that records candidate and commit state.
func (rec *Recorder) Instance(virtualgw string) dispatcher.Validator {
	instance := &instance{
		logger:     rec.Logger,
		enumerator: rec.Enumerator,
		virtualgw:  virtualgw,
		validator:  rec.ValidatorFactory.Instance(virtualgw),
	}
	rec.instances[virtualgw] = instance
	return instance
}

// Reset clears transient validation state for a run.
func (i *instance) Reset() {
	i.candidate = nil
	i.isYaml = false
	i.validator.Reset()
}

// Validate records the candidate payload and delegates validation.
func (i *instance) Validate(candidate []byte, isYAML bool) (bool, error) {
	i.candidate = candidate
	i.isYaml = isYAML
	return i.validator.Validate(candidate, isYAML)
}

// Commit stores the last validated candidate for querying
func (i *instance) Commit() {
	defer i.validator.Commit()

	// Parse the generated config
	var result compiler.Snippet
	if i.isYaml {
		if err := yaml.Unmarshal(i.candidate, &result.Data); err != nil {
			i.logger.Error("unmarshal yaml", "error", err)
			return
		}
	} else {
		if err := json.Unmarshal(i.candidate, &result.Data); err != nil {
			i.logger.Error("unmarshal json", "error", err)
			return
		}
	}
	// Collect route snippets and descriptions
	snippets := i.enumerator.Snippets(i.virtualgw)
	root := compiler.DefaultMergingRules()
	index := indexSnippets(root, snippets)
	descriptions := describeConfigs(root, result, index)
	i.WithLock(func() {
		i.snippets = snippets
		i.index = descriptions
	})
}

// describeConfigs builds a Descriptor from validated config and source index
//   - We use the validated config for tags. That ay, tags can have actual values,
//     like domain names or paths, after environment substitution.
//   - However, we use the source index to get the paths of contributing sources.
//     when returning the config of a particular object, we don't want to return the
//     final config, that might contain secrets. Instead, we will return a merge of
//     the original sources before environment substitution.
//
// It is recommended to cache results by LastModified timestamp to avoid unnecessary recomputation.
func describeConfigs(root compiler.MergingRule, config compiler.Snippet, index map[string]map[string][]string) map[string]map[string]Descriptor {
	result := make(map[string]map[string]Descriptor)
	for kind, ids := range root.AsTree(config) {
		if _, exists := result[kind]; !exists {
			result[kind] = make(map[string]Descriptor)
		}
		rule, ruleExists := root.Children[kind]
		ind, indExists := index[kind]
		for id, data := range ids {
			desc := Descriptor{}
			// Append paths of sources that contribute to this id
			if indExists && ind != nil {
				desc.Paths = ind[id]
			}
			// Also append map of tags
			if ruleExists && rule.Tagger != nil {
				tags := rule.Tagger(data)
				if desc.Tags == nil {
					desc.Tags = make(map[string][]string)
				}
				for k, v := range tags {
					desc.Tags[k] = append(desc.Tags[k], v...)
				}
			}
			result[kind][id] = desc
		}
	}
	return result
}

// Index extracts IDs of all sources, and returns an index (kind, id) => source key
// It is recommended to cache results by LastModified timestamp to avoid unnecessary recomputation.
func indexSnippets(root compiler.MergingRule, sources map[string]compiler.Snippet) map[string]map[string][]string {
	index := make(map[string]map[string][]string)
	for path, snippet := range sources {
		mapping := root.AsTree(snippet)
		if mapping != nil {
			for kind, ids := range mapping {
				if _, exists := index[kind]; !exists {
					index[kind] = make(map[string][]string)
				}
				for id := range ids {
					// Append list of paths that contribute to this id
					desc := append(index[kind][id], path)
					index[kind][id] = desc
				}
			}
		}
	}
	return index
}
