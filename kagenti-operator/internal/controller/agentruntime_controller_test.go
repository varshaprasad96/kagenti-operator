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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/kagenti/operator/api/v1alpha1"
)

var _ = Describe("AgentRuntime Controller", func() {

	const (
		deploymentName = "test-runtime-agent"
		runtimeName    = "test-agentruntime"
		namespace      = "default"
	)

	ctx := context.Background()

	runtimeNN := types.NamespacedName{
		Name:      runtimeName,
		Namespace: namespace,
	}

	var reconciler *AgentRuntimeReconciler

	createDeployment := func(name string) {
		replicas := int32(1)
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": name,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app.kubernetes.io/name": name,
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "agent",
								Image: "test-image:latest",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
	}

	createAgentRuntime := func(name, targetName string, runtimeType agentv1alpha1.RuntimeType) {
		rt := &agentv1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: agentv1alpha1.AgentRuntimeSpec{
				Type: runtimeType,
				TargetRef: agentv1alpha1.TargetRef{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       targetName,
				},
			},
		}
		Expect(k8sClient.Create(ctx, rt)).To(Succeed())
	}

	reconcileRT := func() (reconcile.Result, error) {
		return reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: runtimeNN})
	}

	BeforeEach(func() {
		reconciler = &AgentRuntimeReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	Context("When creating an AgentRuntime with a valid Deployment target", func() {
		BeforeEach(func() {
			createDeployment(deploymentName)
			createAgentRuntime(runtimeName, deploymentName, agentv1alpha1.RuntimeTypeAgent)
		})

		AfterEach(func() {
			// Clean up AgentRuntime (remove finalizer first)
			rt := &agentv1alpha1.AgentRuntime{}
			if err := k8sClient.Get(ctx, runtimeNN, rt); err == nil {
				rt.Finalizers = nil
				_ = k8sClient.Update(ctx, rt)
				_ = k8sClient.Delete(ctx, rt)
			}
			// Clean up Deployment
			dep := &appsv1.Deployment{}
			depNN := types.NamespacedName{Name: deploymentName, Namespace: namespace}
			if err := k8sClient.Get(ctx, depNN, dep); err == nil {
				_ = k8sClient.Delete(ctx, dep)
			}
		})

		It("should add finalizer on first reconcile", func() {
			_, err := reconcileRT()
			Expect(err).NotTo(HaveOccurred())

			rt := &agentv1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, runtimeNN, rt)).To(Succeed())
			Expect(rt.Finalizers).To(ContainElement(AgentRuntimeFinalizer))
		})

		It("should apply labels and config-hash to the target Deployment", func() {
			By("first reconcile adds finalizer")
			_, err := reconcileRT()
			Expect(err).NotTo(HaveOccurred())

			By("second reconcile applies config")
			_, err = reconcileRT()
			Expect(err).NotTo(HaveOccurred())

			By("verifying workload metadata labels")
			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: namespace}, dep)).To(Succeed())
			Expect(dep.Labels[LabelAgentType]).To(Equal("agent"))
			Expect(dep.Labels[LabelManagedBy]).To(Equal(LabelManagedByValue))

			By("verifying PodTemplateSpec labels")
			Expect(dep.Spec.Template.Labels[LabelAgentType]).To(Equal("agent"))

			By("verifying PodTemplateSpec config-hash annotation")
			Expect(dep.Spec.Template.Annotations).To(HaveKey(LabelConfigHash))
			Expect(dep.Spec.Template.Annotations[LabelConfigHash]).NotTo(BeEmpty())
		})

		It("should set status to Active after successful reconcile", func() {
			By("reconcile twice: finalizer + config")
			_, _ = reconcileRT()
			_, err := reconcileRT()
			Expect(err).NotTo(HaveOccurred())

			rt := &agentv1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, runtimeNN, rt)).To(Succeed())
			Expect(rt.Status.Phase).To(Equal(agentv1alpha1.RuntimePhaseActive))
		})

		It("should update config-hash when spec changes", func() {
			By("reconcile twice to apply initial config")
			_, _ = reconcileRT()
			_, _ = reconcileRT()

			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: namespace}, dep)).To(Succeed())
			originalHash := dep.Spec.Template.Annotations[LabelConfigHash]

			By("updating AgentRuntime spec with trace config")
			rt := &agentv1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, runtimeNN, rt)).To(Succeed())
			rt.Spec.Trace = &agentv1alpha1.TraceSpec{
				Endpoint: "otel:4317",
				Protocol: agentv1alpha1.TraceProtocolGRPC,
			}
			Expect(k8sClient.Update(ctx, rt)).To(Succeed())

			By("reconciling with updated spec")
			_, err := reconcileRT()
			Expect(err).NotTo(HaveOccurred())

			By("verifying config-hash changed")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: namespace}, dep)).To(Succeed())
			Expect(dep.Spec.Template.Annotations[LabelConfigHash]).NotTo(Equal(originalHash))
		})
	})

	Context("When the target workload does not exist", func() {
		BeforeEach(func() {
			createAgentRuntime(runtimeName, "nonexistent-deployment", agentv1alpha1.RuntimeTypeAgent)
		})

		AfterEach(func() {
			rt := &agentv1alpha1.AgentRuntime{}
			if err := k8sClient.Get(ctx, runtimeNN, rt); err == nil {
				rt.Finalizers = nil
				_ = k8sClient.Update(ctx, rt)
				_ = k8sClient.Delete(ctx, rt)
			}
		})

		It("should set status to Error with TargetNotFound condition", func() {
			By("first reconcile adds finalizer")
			_, _ = reconcileRT()

			By("second reconcile detects missing target")
			result, err := reconcileRT()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			rt := &agentv1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, runtimeNN, rt)).To(Succeed())
			Expect(rt.Status.Phase).To(Equal(agentv1alpha1.RuntimePhaseError))

			// Check condition
			var targetCondition *metav1.Condition
			for i := range rt.Status.Conditions {
				if rt.Status.Conditions[i].Type == ConditionTypeTargetResolved {
					targetCondition = &rt.Status.Conditions[i]
					break
				}
			}
			Expect(targetCondition).NotTo(BeNil())
			Expect(targetCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(targetCondition.Reason).To(Equal("TargetNotFound"))
		})
	})

	Context("When deleting an AgentRuntime", func() {
		BeforeEach(func() {
			createDeployment(deploymentName)
			createAgentRuntime(runtimeName, deploymentName, agentv1alpha1.RuntimeTypeAgent)
		})

		AfterEach(func() {
			dep := &appsv1.Deployment{}
			depNN := types.NamespacedName{Name: deploymentName, Namespace: namespace}
			if err := k8sClient.Get(ctx, depNN, dep); err == nil {
				_ = k8sClient.Delete(ctx, dep)
			}
		})

		It("should preserve type label and update config-hash on deletion", func() {
			By("reconcile twice to apply config")
			_, _ = reconcileRT()
			_, _ = reconcileRT()

			dep := &appsv1.Deployment{}
			depNN := types.NamespacedName{Name: deploymentName, Namespace: namespace}
			Expect(k8sClient.Get(ctx, depNN, dep)).To(Succeed())
			originalHash := dep.Spec.Template.Annotations[LabelConfigHash]
			Expect(originalHash).NotTo(BeEmpty())

			By("deleting the AgentRuntime")
			rt := &agentv1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, runtimeNN, rt)).To(Succeed())
			Expect(k8sClient.Delete(ctx, rt)).To(Succeed())

			By("reconciling the deletion")
			_, err := reconcileRT()
			Expect(err).NotTo(HaveOccurred())

			By("verifying type label is preserved on PodTemplateSpec")
			Expect(k8sClient.Get(ctx, depNN, dep)).To(Succeed())
			Expect(dep.Spec.Template.Labels[LabelAgentType]).To(Equal("agent"))

			By("verifying config-hash changed to defaults-only")
			newHash := dep.Spec.Template.Annotations[LabelConfigHash]
			Expect(newHash).NotTo(Equal(originalHash))

			By("verifying managed-by label is removed")
			Expect(dep.Labels).NotTo(HaveKey(LabelManagedBy))
		})
	})

	Context("When creating a tool runtime", func() {
		const toolDeploymentName = "test-runtime-tool"
		const toolRuntimeName = "test-tool-runtime"

		toolRuntimeNN := types.NamespacedName{
			Name:      toolRuntimeName,
			Namespace: namespace,
		}

		BeforeEach(func() {
			createDeployment(toolDeploymentName)
			rt := &agentv1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      toolRuntimeName,
					Namespace: namespace,
				},
				Spec: agentv1alpha1.AgentRuntimeSpec{
					Type: agentv1alpha1.RuntimeTypeTool,
					TargetRef: agentv1alpha1.TargetRef{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       toolDeploymentName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rt)).To(Succeed())
		})

		AfterEach(func() {
			rt := &agentv1alpha1.AgentRuntime{}
			if err := k8sClient.Get(ctx, toolRuntimeNN, rt); err == nil {
				rt.Finalizers = nil
				_ = k8sClient.Update(ctx, rt)
				_ = k8sClient.Delete(ctx, rt)
			}
			dep := &appsv1.Deployment{}
			depNN := types.NamespacedName{Name: toolDeploymentName, Namespace: namespace}
			if err := k8sClient.Get(ctx, depNN, dep); err == nil {
				_ = k8sClient.Delete(ctx, dep)
			}
		})

		It("should apply tool type label", func() {
			toolReconciler := &AgentRuntimeReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("reconcile twice: finalizer + config")
			_, _ = toolReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: toolRuntimeNN})
			_, err := toolReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: toolRuntimeNN})
			Expect(err).NotTo(HaveOccurred())

			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: toolDeploymentName, Namespace: namespace}, dep)).To(Succeed())
			Expect(dep.Labels[LabelAgentType]).To(Equal("tool"))
			Expect(dep.Spec.Template.Labels[LabelAgentType]).To(Equal("tool"))
		})
	})
})
