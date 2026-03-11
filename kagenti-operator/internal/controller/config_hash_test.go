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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentv1alpha1 "github.com/kagenti/operator/api/v1alpha1"
)

func newFakeClientWithDefaults(data map[string]string) *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = agentv1alpha1.AddToScheme(scheme)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	if data != nil {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      DefaultsConfigMapName,
				Namespace: DefaultsConfigMapNamespace,
			},
			Data: data,
		}
		builder = builder.WithObjects(cm)
	}
	return builder
}

func TestComputeConfigHash_Deterministic(t *testing.T) {
	c := newFakeClientWithDefaults(map[string]string{"otel-endpoint": "collector:4317"}).Build()
	ctx := context.Background()

	spec := &agentv1alpha1.AgentRuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeAgent,
		TargetRef: agentv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-agent",
		},
		Trace: &agentv1alpha1.TraceSpec{
			Endpoint: "otel:4317",
			Protocol: agentv1alpha1.TraceProtocolGRPC,
			Sampling: &agentv1alpha1.SamplingSpec{Rate: 0.5},
		},
	}

	hash1, err := ComputeConfigHash(ctx, c, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash2, err := ComputeConfigHash(ctx, c, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("expected deterministic hashes, got %s and %s", hash1, hash2)
	}
}

func TestComputeConfigHash_ChangesOnSpecChange(t *testing.T) {
	c := newFakeClientWithDefaults(map[string]string{}).Build()
	ctx := context.Background()

	spec1 := &agentv1alpha1.AgentRuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeAgent,
		TargetRef: agentv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-agent",
		},
	}

	spec2 := &agentv1alpha1.AgentRuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeTool,
		TargetRef: agentv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-agent",
		},
	}

	hash1, err := ComputeConfigHash(ctx, c, spec1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash2, err := ComputeConfigHash(ctx, c, spec2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("expected different hashes for different specs")
	}
}

func TestComputeConfigHash_ChangesOnTraceChange(t *testing.T) {
	c := newFakeClientWithDefaults(map[string]string{}).Build()
	ctx := context.Background()

	spec1 := &agentv1alpha1.AgentRuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeAgent,
		TargetRef: agentv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-agent",
		},
		Trace: &agentv1alpha1.TraceSpec{
			Endpoint: "otel:4317",
			Protocol: agentv1alpha1.TraceProtocolGRPC,
		},
	}

	spec2 := &agentv1alpha1.AgentRuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeAgent,
		TargetRef: agentv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-agent",
		},
		Trace: &agentv1alpha1.TraceSpec{
			Endpoint: "otel:4318",
			Protocol: agentv1alpha1.TraceProtocolHTTP,
		},
	}

	hash1, err := ComputeConfigHash(ctx, c, spec1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash2, err := ComputeConfigHash(ctx, c, spec2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("expected different hashes for different trace config")
	}
}

func TestComputeConfigHash_ChangesOnIdentityChange(t *testing.T) {
	c := newFakeClientWithDefaults(map[string]string{}).Build()
	ctx := context.Background()

	spec1 := &agentv1alpha1.AgentRuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeAgent,
		TargetRef: agentv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-agent",
		},
		Identity: &agentv1alpha1.IdentitySpec{
			SPIFFE: &agentv1alpha1.SPIFFEIdentity{
				TrustDomain: "example.org",
			},
		},
	}

	spec2 := &agentv1alpha1.AgentRuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeAgent,
		TargetRef: agentv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-agent",
		},
		Identity: &agentv1alpha1.IdentitySpec{
			SPIFFE: &agentv1alpha1.SPIFFEIdentity{
				TrustDomain: "other.org",
			},
		},
	}

	hash1, err := ComputeConfigHash(ctx, c, spec1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash2, err := ComputeConfigHash(ctx, c, spec2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("expected different hashes for different trust domains")
	}
}

func TestComputeDefaultsOnlyHash_DiffersFromSpecHash(t *testing.T) {
	defaults := map[string]string{"otel-endpoint": "default-collector:4317"}
	c := newFakeClientWithDefaults(defaults).Build()
	ctx := context.Background()

	spec := &agentv1alpha1.AgentRuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeAgent,
		TargetRef: agentv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-agent",
		},
	}

	specHash, err := ComputeConfigHash(ctx, c, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defaultsHash, err := ComputeDefaultsOnlyHash(ctx, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if specHash == defaultsHash {
		t.Error("expected defaults-only hash to differ from spec+defaults hash")
	}
}

func TestComputeDefaultsOnlyHash_Deterministic(t *testing.T) {
	defaults := map[string]string{
		"otel-endpoint": "collector:4317",
		"otel-protocol": "grpc",
	}
	c := newFakeClientWithDefaults(defaults).Build()
	ctx := context.Background()

	hash1, err := ComputeDefaultsOnlyHash(ctx, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash2, err := ComputeDefaultsOnlyHash(ctx, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("expected deterministic hashes, got %s and %s", hash1, hash2)
	}
}

func TestComputeConfigHash_MissingConfigMap(t *testing.T) {
	// No ConfigMap created — should still produce a hash using empty defaults
	c := newFakeClientWithDefaults(nil).Build()
	ctx := context.Background()

	spec := &agentv1alpha1.AgentRuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeAgent,
		TargetRef: agentv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-agent",
		},
	}

	hash, err := ComputeConfigHash(ctx, c, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash == "" {
		t.Error("expected non-empty hash even with missing ConfigMap")
	}
}

func TestComputeConfigHash_ChangesOnDefaultsChange(t *testing.T) {
	ctx := context.Background()

	c1 := newFakeClientWithDefaults(map[string]string{"key": "value1"}).Build()
	c2 := newFakeClientWithDefaults(map[string]string{"key": "value2"}).Build()

	spec := &agentv1alpha1.AgentRuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeAgent,
		TargetRef: agentv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-agent",
		},
	}

	hash1, err := ComputeConfigHash(ctx, c1, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash2, err := ComputeConfigHash(ctx, c2, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("expected different hashes when defaults change")
	}
}

func TestBuildConfigData_NilSpec(t *testing.T) {
	defaults := map[string]string{"key": "value"}
	data := buildConfigData(nil, defaults)

	if data.Type != "" {
		t.Errorf("expected empty type for nil spec, got %s", data.Type)
	}
	if data.Trace != nil {
		t.Error("expected nil trace for nil spec")
	}
	if data.TrustDomain != "" {
		t.Errorf("expected empty trust domain for nil spec, got %s", data.TrustDomain)
	}
	if data.Defaults["key"] != "value" {
		t.Error("expected defaults to be preserved with nil spec")
	}
}

func TestBuildConfigData_FullSpec(t *testing.T) {
	spec := &agentv1alpha1.AgentRuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeAgent,
		TargetRef: agentv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "my-agent",
		},
		Identity: &agentv1alpha1.IdentitySpec{
			SPIFFE: &agentv1alpha1.SPIFFEIdentity{
				TrustDomain: "example.org",
			},
		},
		Trace: &agentv1alpha1.TraceSpec{
			Endpoint: "otel:4317",
			Protocol: agentv1alpha1.TraceProtocolGRPC,
			Sampling: &agentv1alpha1.SamplingSpec{Rate: 0.5},
		},
	}

	data := buildConfigData(spec, map[string]string{})

	if data.Type != "agent" {
		t.Errorf("expected type=agent, got %s", data.Type)
	}
	if data.TrustDomain != "example.org" {
		t.Errorf("expected trustDomain=example.org, got %s", data.TrustDomain)
	}
	if data.Trace == nil {
		t.Fatal("expected trace to be set")
	}
	if data.Trace.Endpoint != "otel:4317" {
		t.Errorf("expected endpoint=otel:4317, got %s", data.Trace.Endpoint)
	}
	if data.Trace.Protocol != "grpc" {
		t.Errorf("expected protocol=grpc, got %s", data.Trace.Protocol)
	}
	if data.Trace.Rate != 0.5 {
		t.Errorf("expected rate=0.5, got %f", data.Trace.Rate)
	}
}
