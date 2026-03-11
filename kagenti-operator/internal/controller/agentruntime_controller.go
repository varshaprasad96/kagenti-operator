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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/kagenti/operator/api/v1alpha1"
)

const (
	AgentRuntimeFinalizer = "kagenti.io/cleanup"

	// LabelConfigHash is the annotation applied to PodTemplateSpec to trigger rolling updates.
	LabelConfigHash = "kagenti.io/config-hash"

	// Condition types for AgentRuntime status.
	ConditionTypeReady          = "Ready"
	ConditionTypeTargetResolved = "TargetResolved"
)

// AgentRuntimeReconciler reconciles AgentRuntime objects.
type AgentRuntimeReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=agent.kagenti.dev,resources=agentruntimes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.kagenti.dev,resources=agentruntimes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.kagenti.dev,resources=agentruntimes/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

func (r *AgentRuntimeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Reconciling AgentRuntime", "namespacedName", req.NamespacedName)

	// 1. Fetch the AgentRuntime CR
	rt := &agentv1alpha1.AgentRuntime{}
	if err := r.Get(ctx, req.NamespacedName, rt); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Handle deletion
	if !rt.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, rt)
	}

	// 3. Ensure finalizer
	if !controllerutil.ContainsFinalizer(rt, AgentRuntimeFinalizer) {
		controllerutil.AddFinalizer(rt, AgentRuntimeFinalizer)
		if err := r.Update(ctx, rt); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 4. Resolve targetRef
	workloadObj, err := r.resolveTargetRef(ctx, rt)
	if err != nil {
		logger.Error(err, "Failed to resolve targetRef")
		r.setPhase(ctx, rt, agentv1alpha1.RuntimePhaseError)
		r.setCondition(rt, ConditionTypeTargetResolved, metav1.ConditionFalse, "TargetNotFound", err.Error())
		if statusErr := r.Status().Update(ctx, rt); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}
		if r.Recorder != nil {
			r.Recorder.Event(rt, corev1.EventTypeWarning, "TargetNotFound", err.Error())
		}
		return ctrl.Result{RequeueAfter: 30 * 1e9}, nil // 30s
	}

	r.setCondition(rt, ConditionTypeTargetResolved, metav1.ConditionTrue, "TargetFound",
		fmt.Sprintf("%s %s resolved", rt.Spec.TargetRef.Kind, rt.Spec.TargetRef.Name))

	// 5. Compute config hash
	configHash, err := ComputeConfigHash(ctx, r.Client, &rt.Spec)
	if err != nil {
		logger.Error(err, "Failed to compute config hash")
		r.setPhase(ctx, rt, agentv1alpha1.RuntimePhaseError)
		r.setCondition(rt, ConditionTypeReady, metav1.ConditionFalse, "ConfigHashError", err.Error())
		if statusErr := r.Status().Update(ctx, rt); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	// 6. Apply labels and annotations to the target workload
	if err := r.applyWorkloadConfig(ctx, rt, workloadObj, configHash); err != nil {
		logger.Error(err, "Failed to apply workload config")
		r.setPhase(ctx, rt, agentv1alpha1.RuntimePhaseError)
		r.setCondition(rt, ConditionTypeReady, metav1.ConditionFalse, "ConfigApplyError", err.Error())
		if statusErr := r.Status().Update(ctx, rt); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	// 7. Count configured pods
	configuredPods, err := r.countConfiguredPods(ctx, rt)
	if err != nil {
		logger.V(1).Info("Failed to count configured pods", "error", err)
	}

	// 8. Update status
	rt.Status.ConfiguredPods = configuredPods
	r.setPhase(ctx, rt, agentv1alpha1.RuntimePhaseActive)
	r.setCondition(rt, ConditionTypeReady, metav1.ConditionTrue, "Configured",
		fmt.Sprintf("Workload %s configured with config-hash %s", rt.Spec.TargetRef.Name, configHash[:12]))
	if err := r.Status().Update(ctx, rt); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	if r.Recorder != nil {
		r.Recorder.Event(rt, corev1.EventTypeNormal, "Configured",
			fmt.Sprintf("Applied config to %s %s", rt.Spec.TargetRef.Kind, rt.Spec.TargetRef.Name))
	}

	return ctrl.Result{}, nil
}

// resolveTargetRef looks up the workload referenced by spec.targetRef.
func (r *AgentRuntimeReconciler) resolveTargetRef(ctx context.Context, rt *agentv1alpha1.AgentRuntime) (*unstructured.Unstructured, error) {
	ref := rt.Spec.TargetRef

	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid apiVersion %s: %w", ref.APIVersion, err)
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gv.WithKind(ref.Kind))

	key := client.ObjectKey{Namespace: rt.Namespace, Name: ref.Name}
	if err := r.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%s/%s %s not found in namespace %s", ref.APIVersion, ref.Kind, ref.Name, rt.Namespace)
		}
		return nil, err
	}

	return obj, nil
}

// applyWorkloadConfig applies kagenti labels and config-hash annotation to the
// target workload's metadata and PodTemplateSpec.
func (r *AgentRuntimeReconciler) applyWorkloadConfig(ctx context.Context, rt *agentv1alpha1.AgentRuntime, _ *unstructured.Unstructured, configHash string) error {
	logger := log.FromContext(ctx)
	ref := rt.Spec.TargetRef

	acc, ok := newRuntimePodTemplateAccessor(ref.Kind)
	if !ok {
		return fmt.Errorf("unsupported workload kind: %s", ref.Kind)
	}

	key := types.NamespacedName{Name: ref.Name, Namespace: rt.Namespace}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Get(ctx, key, acc.obj); err != nil {
			return err
		}

		// Apply labels to workload metadata
		workloadLabels := acc.obj.GetLabels()
		if workloadLabels == nil {
			workloadLabels = make(map[string]string)
		}
		workloadLabels[LabelAgentType] = string(rt.Spec.Type)
		workloadLabels[LabelManagedBy] = LabelManagedByValue
		acc.obj.SetLabels(workloadLabels)

		// Apply labels to PodTemplateSpec
		podLabels := acc.getPodLabels(acc.obj)
		if podLabels == nil {
			podLabels = make(map[string]string)
		}
		podLabels[LabelAgentType] = string(rt.Spec.Type)
		acc.setPodLabels(acc.obj, podLabels)

		// Apply config-hash annotation to PodTemplateSpec
		podAnnotations := acc.getPodAnnotations(acc.obj)
		if podAnnotations == nil {
			podAnnotations = make(map[string]string)
		}

		currentHash := podAnnotations[LabelConfigHash]
		if currentHash == configHash &&
			workloadLabels[LabelAgentType] == string(rt.Spec.Type) {
			// No changes needed
			return nil
		}

		podAnnotations[LabelConfigHash] = configHash
		acc.setPodAnnotations(acc.obj, podAnnotations)

		logger.Info("Applying config to workload",
			"workload", ref.Name,
			"kind", ref.Kind,
			"type", string(rt.Spec.Type),
			"configHash", configHash[:12])

		return r.Update(ctx, acc.obj)
	})
}

// countConfiguredPods counts pods that have the kagenti.io/type label matching the runtime type.
func (r *AgentRuntimeReconciler) countConfiguredPods(ctx context.Context, rt *agentv1alpha1.AgentRuntime) (int32, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(rt.Namespace),
		client.MatchingLabels{LabelAgentType: string(rt.Spec.Type)},
	); err != nil {
		return 0, err
	}

	// Filter to pods belonging to the target workload by checking owner references
	var count int32
	for i := range podList.Items {
		pod := &podList.Items[i]
		if isPodOwnedByWorkload(pod, rt.Spec.TargetRef.Name) {
			count++
		}
	}
	return count, nil
}

// isPodOwnedByWorkload checks if a pod is transitively owned by the named workload.
// For Deployments, the chain is: Deployment -> ReplicaSet -> Pod.
func isPodOwnedByWorkload(pod *corev1.Pod, workloadName string) bool {
	for _, ref := range pod.OwnerReferences {
		// ReplicaSet names are prefixed with the Deployment name
		if ref.Kind == "ReplicaSet" && len(ref.Name) > len(workloadName) &&
			ref.Name[:len(workloadName)+1] == workloadName+"-" {
			return true
		}
		// StatefulSet pods are directly owned
		if ref.Kind == "StatefulSet" && ref.Name == workloadName {
			return true
		}
	}
	return false
}

// handleDeletion runs finalizer logic when an AgentRuntime is deleted.
// It preserves the kagenti.io/type label and updates the config-hash to defaults-only.
func (r *AgentRuntimeReconciler) handleDeletion(ctx context.Context, rt *agentv1alpha1.AgentRuntime) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(rt, AgentRuntimeFinalizer) {
		return ctrl.Result{}, nil
	}

	logger.Info("Handling AgentRuntime deletion", "name", rt.Name)

	// Update config-hash to defaults-only to trigger rolling update
	ref := rt.Spec.TargetRef
	acc, ok := newRuntimePodTemplateAccessor(ref.Kind)
	if ok {
		defaultsHash, err := ComputeDefaultsOnlyHash(ctx, r.Client)
		if err != nil {
			logger.V(1).Info("Failed to compute defaults-only hash, using empty", "error", err)
			defaultsHash = ""
		}

		key := types.NamespacedName{Name: ref.Name, Namespace: rt.Namespace}
		updateErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.Get(ctx, key, acc.obj); err != nil {
				return client.IgnoreNotFound(err)
			}

			// Preserve kagenti.io/type label (per issue spec)
			// Update config-hash to defaults-only
			podAnnotations := acc.getPodAnnotations(acc.obj)
			if podAnnotations == nil {
				podAnnotations = make(map[string]string)
			}
			podAnnotations[LabelConfigHash] = defaultsHash
			acc.setPodAnnotations(acc.obj, podAnnotations)

			// Remove managed-by from workload metadata
			workloadLabels := acc.obj.GetLabels()
			delete(workloadLabels, LabelManagedBy)
			acc.obj.SetLabels(workloadLabels)

			logger.Info("Updated workload to defaults-only config on AgentRuntime deletion",
				"workload", ref.Name, "kind", ref.Kind)
			return r.Update(ctx, acc.obj)
		})
		if updateErr != nil {
			logger.Error(updateErr, "Failed to update workload on deletion")
		}
	}

	// Remove finalizer
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &agentv1alpha1.AgentRuntime{}
		if err := r.Get(ctx, types.NamespacedName{Name: rt.Name, Namespace: rt.Namespace}, latest); err != nil {
			return err
		}
		controllerutil.RemoveFinalizer(latest, AgentRuntimeFinalizer)
		return r.Update(ctx, latest)
	}); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Removed finalizer from AgentRuntime", "name", rt.Name)
	return ctrl.Result{}, nil
}

// setPhase sets the status phase on the AgentRuntime (does not persist — caller must update).
func (r *AgentRuntimeReconciler) setPhase(_ context.Context, rt *agentv1alpha1.AgentRuntime, phase agentv1alpha1.RuntimePhase) {
	rt.Status.Phase = phase
}

// setCondition sets a condition on the AgentRuntime status (does not persist — caller must update).
func (r *AgentRuntimeReconciler) setCondition(rt *agentv1alpha1.AgentRuntime, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&rt.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: rt.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// mapWorkloadToAgentRuntime returns a handler.MapFunc that maps workload events
// to AgentRuntime reconcile requests.
func (r *AgentRuntimeReconciler) mapWorkloadToAgentRuntime(apiVersion, kind string) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		rtList := &agentv1alpha1.AgentRuntimeList{}
		if err := r.List(ctx, rtList, client.InNamespace(obj.GetNamespace())); err != nil {
			return nil
		}

		var requests []reconcile.Request
		for _, rt := range rtList.Items {
			if rt.Spec.TargetRef.Name == obj.GetName() &&
				rt.Spec.TargetRef.Kind == kind &&
				rt.Spec.TargetRef.APIVersion == apiVersion {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      rt.Name,
						Namespace: rt.Namespace,
					},
				})
			}
		}
		return requests
	}
}

// SetupWithManager registers the AgentRuntime controller with the manager.
func (r *AgentRuntimeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.AgentRuntime{}).
		Watches(
			&appsv1.Deployment{},
			handler.EnqueueRequestsFromMapFunc(r.mapWorkloadToAgentRuntime("apps/v1", "Deployment")),
		).
		Watches(
			&appsv1.StatefulSet{},
			handler.EnqueueRequestsFromMapFunc(r.mapWorkloadToAgentRuntime("apps/v1", "StatefulSet")),
		).
		Named("AgentRuntime").
		Complete(r)
}

// runtimePodTemplateAccessor provides uniform access to PodTemplateSpec
// labels and annotations across Deployment and StatefulSet.
type runtimePodTemplateAccessor struct {
	obj               client.Object
	getPodLabels      func(client.Object) map[string]string
	setPodLabels      func(client.Object, map[string]string)
	getPodAnnotations func(client.Object) map[string]string
	setPodAnnotations func(client.Object, map[string]string)
}

func newRuntimePodTemplateAccessor(kind string) (*runtimePodTemplateAccessor, bool) {
	switch kind {
	case "Deployment":
		return &runtimePodTemplateAccessor{
			obj: &appsv1.Deployment{},
			getPodLabels: func(o client.Object) map[string]string {
				return o.(*appsv1.Deployment).Spec.Template.Labels
			},
			setPodLabels: func(o client.Object, l map[string]string) {
				o.(*appsv1.Deployment).Spec.Template.Labels = l
			},
			getPodAnnotations: func(o client.Object) map[string]string {
				return o.(*appsv1.Deployment).Spec.Template.Annotations
			},
			setPodAnnotations: func(o client.Object, a map[string]string) {
				o.(*appsv1.Deployment).Spec.Template.Annotations = a
			},
		}, true
	case "StatefulSet":
		return &runtimePodTemplateAccessor{
			obj: &appsv1.StatefulSet{},
			getPodLabels: func(o client.Object) map[string]string {
				return o.(*appsv1.StatefulSet).Spec.Template.Labels
			},
			setPodLabels: func(o client.Object, l map[string]string) {
				o.(*appsv1.StatefulSet).Spec.Template.Labels = l
			},
			getPodAnnotations: func(o client.Object) map[string]string {
				return o.(*appsv1.StatefulSet).Spec.Template.Annotations
			},
			setPodAnnotations: func(o client.Object, a map[string]string) {
				o.(*appsv1.StatefulSet).Spec.Template.Annotations = a
			},
		}, true
	default:
		return nil, false
	}
}
