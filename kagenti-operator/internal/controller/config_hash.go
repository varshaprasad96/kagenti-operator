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
		Defaults: defaults,
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

// hashConfigData produces a deterministic SHA256 hex string from configData.
// Go's encoding/json sorts map keys by default, ensuring deterministic output.
func hashConfigData(data configData) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config data: %w", err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
