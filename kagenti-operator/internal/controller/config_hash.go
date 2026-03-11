/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/kagenti/operator/api/v1alpha1"
)

const (
	// DefaultsConfigMapName is the ConfigMap containing platform webhook defaults.
	DefaultsConfigMapName = "kagenti-webhook-defaults"

	// DefaultsConfigMapNamespace is the namespace where the defaults ConfigMap lives.
	DefaultsConfigMapNamespace = "kagenti-webhook-system"
)

// configData is the canonical representation used for hash computation.
// Fields are exported for JSON serialization.
type configData struct {
	Type        string            `json:"type"`
	TrustDomain string            `json:"trustDomain,omitempty"`
	Trace       *traceConfigData  `json:"trace,omitempty"`
	Defaults    map[string]string `json:"defaults,omitempty"`
}

type traceConfigData struct {
	Endpoint string  `json:"endpoint,omitempty"`
	Protocol string  `json:"protocol,omitempty"`
	Rate     float64 `json:"rate,omitempty"`
}

// ComputeConfigHash computes a deterministic SHA256 hash from an AgentRuntime spec
// merged with platform defaults from the ConfigMap.
func ComputeConfigHash(ctx context.Context, c client.Reader, spec *agentv1alpha1.AgentRuntimeSpec) (string, error) {
	defaults, err := readDefaults(ctx, c)
	if err != nil {
		defaults = map[string]string{}
	}

	data := buildConfigData(spec, defaults)
	return hashConfigData(data)
}

// ComputeDefaultsOnlyHash computes a hash using only platform defaults (no spec overrides).
// Used when an AgentRuntime is deleted to trigger a rolling update back to defaults.
func ComputeDefaultsOnlyHash(ctx context.Context, c client.Reader) (string, error) {
	defaults, err := readDefaults(ctx, c)
	if err != nil {
		defaults = map[string]string{}
	}

	data := buildConfigData(nil, defaults)
	return hashConfigData(data)
}

// readDefaults reads the platform defaults ConfigMap.
func readDefaults(ctx context.Context, c client.Reader) (map[string]string, error) {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      DefaultsConfigMapName,
		Namespace: DefaultsConfigMapNamespace,
	}
	if err := c.Get(ctx, key, cm); err != nil {
		return nil, fmt.Errorf("failed to read defaults ConfigMap %s/%s: %w", DefaultsConfigMapNamespace, DefaultsConfigMapName, err)
	}
	return cm.Data, nil
}

// buildConfigData merges an AgentRuntime spec with platform defaults into
// a canonical configData struct. If spec is nil, only defaults are included.
func buildConfigData(spec *agentv1alpha1.AgentRuntimeSpec, defaults map[string]string) configData {
	data := configData{
		Defaults: sortedDefaults(defaults),
	}

	if spec == nil {
		return data
	}

	data.Type = string(spec.Type)

	if spec.Identity != nil && spec.Identity.SPIFFE != nil {
		data.TrustDomain = spec.Identity.SPIFFE.TrustDomain
	}

	if spec.Trace != nil {
		data.Trace = &traceConfigData{
			Endpoint: spec.Trace.Endpoint,
			Protocol: string(spec.Trace.Protocol),
		}
		if spec.Trace.Sampling != nil {
			data.Trace.Rate = spec.Trace.Sampling.Rate
		}
	}

	return data
}

// sortedDefaults returns a new map with the same entries. The map itself is
// unordered, but JSON serialization in hashConfigData uses sorted keys.
func sortedDefaults(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// hashConfigData produces a deterministic SHA256 hex string from configData.
// Keys are sorted to ensure stable output.
func hashConfigData(data configData) (string, error) {
	// Use sorted keys for deterministic JSON output
	b, err := marshalSorted(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config data: %w", err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// marshalSorted produces JSON with sorted map keys for deterministic hashing.
func marshalSorted(v interface{}) ([]byte, error) {
	// encoding/json sorts map keys by default in Go, but we explicitly
	// re-marshal through an intermediate map to guarantee ordering for
	// any nested structures.
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	// Unmarshal into ordered structure and re-marshal
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}

	return marshalOrderedMap(m)
}

// marshalOrderedMap recursively marshals a map with sorted keys.
func marshalOrderedMap(m map[string]interface{}) ([]byte, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buf := []byte("{")
	for i, k := range keys {
		if i > 0 {
			buf = append(buf, ',')
		}
		keyJSON, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf = append(buf, keyJSON...)
		buf = append(buf, ':')

		switch val := m[k].(type) {
		case map[string]interface{}:
			valJSON, err := marshalOrderedMap(val)
			if err != nil {
				return nil, err
			}
			buf = append(buf, valJSON...)
		default:
			valJSON, err := json.Marshal(val)
			if err != nil {
				return nil, err
			}
			buf = append(buf, valJSON...)
		}
	}
	buf = append(buf, '}')
	return buf, nil
}
