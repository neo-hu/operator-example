/*
Copyright 2023.

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

package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	webv1 "github.com/neo-hu/operator-example/api/v1"
)

// FrontendReconciler reconciles a Frontend object
type FrontendReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=web.tsdb.top,resources=frontends,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=web.tsdb.top,resources=frontends/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=web.tsdb.top,resources=frontends/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Frontend object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.0/pkg/reconcile
func (r *FrontendReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var frontend webv1.Frontend
	err := r.Get(ctx, req.NamespacedName, &frontend)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	logger.Info("Reconciling Frontend", "images", frontend.Spec.Image)
	// 同步配置文件
	if err = r.applyConfig(ctx, logger, &frontend); err != nil {
		logger.Error(err, "failed to apply frontend config")
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil // 重试
	}
	// 同步crontab
	if err = r.applyCron(ctx, logger, &frontend); err != nil {
		logger.Error(err, "failed to apply frontend cron")
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil // 重试
	}
	// 生成Daemon Sets
	if err = r.applyDaemonSet(ctx, logger, &frontend); err != nil {
		logger.Error(err, "failed to apply frontend daemonset")
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil // 重试
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FrontendReconciler) SetupWithManager(mgr ctrl.Manager) error {
	var filter = predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
				return true
			}
			return false
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&webv1.Frontend{}, builder.WithPredicates(filter)).
		Complete(r)
}

// applyConfig 应用配置文件
func (r *FrontendReconciler) applyConfig(ctx context.Context, logger logr.Logger, f *webv1.Frontend) error {
	var cfg corev1.ConfigMap
	err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: f.Name}, &cfg)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return err
		}
		// 不存在则创建
		err = r.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: defaultObjectMeta(f),
			Data: map[string]string{
				"cfg.yaml": f.Spec.Config,
			},
		})
		if err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("create config"))
		return nil
	}
	if cfg.Data["cfg.yaml"] == f.Spec.Config { // 配置没修改，无需更新
		return nil
	}
	cfg.Data = map[string]string{
		"cfg.yaml": f.Spec.Config,
	}
	if err = r.Update(ctx, &cfg); err != nil {
		return err
	}
	logger.Info("update config")
	return nil
}

// applyDaemonSet 应用DaemonSet
func (r *FrontendReconciler) applyDaemonSet(ctx context.Context, logger logr.Logger, f *webv1.Frontend) error {
	if f.Spec.Image == "" {
		return field.Required(field.NewPath("spec").Child("image"), "image is required")
	}
	daemonSet := &appsv1.DaemonSet{}
	err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace,
		Name: f.Name}, daemonSet)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return err
		}
		// create new daemonSet
		daemonSet = &appsv1.DaemonSet{
			ObjectMeta: defaultObjectMeta(f),
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"operator/name": f.Name},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"operator/name": f.Name},
					},
					Spec: corev1.PodSpec{
						ImagePullSecrets: []corev1.LocalObjectReference{
							{Name: "registry"},
						},
						Volumes: []corev1.Volume{ // 添加配置文件
							{
								Name: "cfg",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: f.Name,
										},
									},
								},
							},
						},
						RestartPolicy: corev1.RestartPolicyAlways,
						NodeSelector:  f.Spec.NodeSelector,
						Containers: []corev1.Container{
							{
								Name:       "frontend",
								Image:      f.Spec.Image,
								Command:    []string{"/api_server", "-c", "/cfg.yaml"},
								WorkingDir: "/",
								Ports: []corev1.ContainerPort{
									{
										HostPort:      8111,
										ContainerPort: 8111,
										Protocol:      corev1.ProtocolTCP,
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "cfg",
										MountPath: "/cfg.yaml",
										ReadOnly:  true,
										SubPath:   "cfg.yaml",
									},
								},
								LivenessProbe: &corev1.Probe{ //健康检查
									ProbeHandler: corev1.ProbeHandler{
										HTTPGet: &corev1.HTTPGetAction{
											Path:   "/healthz",
											Scheme: corev1.URISchemeHTTP,
											Host:   "127.0.0.1",
											Port:   intstr.FromInt(8111)}},
								},
							},
						},
					},
				},
			},
		}
		if err = r.Create(ctx, daemonSet); err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("create daemonSet"))
		return nil
	}
	if daemonSet.Spec.Template.Spec.Containers[0].Image == f.Spec.Image {
		return nil // no change
	}
	daemonSet.Spec.Template.Spec.Containers[0].Image = f.Spec.Image
	if err = r.Update(ctx, daemonSet); err != nil {
		return err
	}
	logger.Info("update daemonSet")
	return nil
}

func (r *FrontendReconciler) applyCron(ctx context.Context, logger logr.Logger, f *webv1.Frontend) error {
	if f.Spec.Image == "" {
		return field.Required(field.NewPath("spec").Child("image"), "image is required")
	}
	err := r.applyStorage(ctx, logger, f)
	if err != nil {
		return err
	}
	job := &v1.CronJob{}
	err = r.Get(ctx, client.ObjectKey{Namespace: f.Namespace,
		Name: f.Name + "-backup-db"}, job)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return err
		}
		job = &v1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      f.Name + "-backup-db",
				Namespace: f.Namespace,
				Labels:    map[string]string{"operator/name": f.Name},
				OwnerReferences: []metav1.OwnerReference{ // 添加关联关系
					*metav1.NewControllerRef(f, schema.GroupVersionKind{
						Group:   webv1.GroupVersion.Group,
						Version: webv1.GroupVersion.Version,
						Kind:    "Frontend",
					}),
				},
			},
			Spec: v1.CronJobSpec{
				Schedule: f.Spec.Backup.Schedule,
				JobTemplate: v1.JobTemplateSpec{
					Spec: v1.JobSpec{Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							NodeSelector:  f.Spec.NodeSelector,
							ImagePullSecrets: []corev1.LocalObjectReference{
								{Name: "registry"},
							},
							Volumes: []corev1.Volume{ // 添加配置文件
								{
									Name: "data",
									VolumeSource: corev1.VolumeSource{
										PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: f.Name,
										},
									},
								},
								{
									Name: "cfg",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: f.Name,
											},
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Image:   f.Spec.Image,
									Name:    "backup-db",
									Command: []string{"/api_server", "-c", "/cfg.yaml", "backup", "--dir", "/data_backup"},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "cfg",
											MountPath: "/cfg.yaml",
											ReadOnly:  true,
											SubPath:   "cfg.yaml",
										},
										{
											Name:      "data",
											MountPath: "/data_backup",
										},
									},
								},
							},
						},
					}},
				},
			},
		}
		if err = r.Create(ctx, job); err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("create cronJob"))
		return nil
	}

	if job.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image == f.Spec.Image &&
		job.Spec.Schedule == f.Spec.Backup.Schedule {
		return nil // no change
	}
	job.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image = f.Spec.Image
	job.Spec.Schedule = f.Spec.Backup.Schedule
	if err = r.Update(ctx, job); err != nil {
		return err
	}
	logger.Info("update cronJob")
	return nil
}

// defaultObjectMeta 默认对象元数据
func defaultObjectMeta(f *webv1.Frontend) metav1.ObjectMeta {
	return metav1.ObjectMeta{Namespace: f.Namespace,
		Name: f.Name, Labels: map[string]string{"operator/name": f.Name},
		OwnerReferences: []metav1.OwnerReference{ // 添加关联关系
			*metav1.NewControllerRef(f, schema.GroupVersionKind{
				Group:   webv1.GroupVersion.Group,
				Version: webv1.GroupVersion.Version,
				Kind:    "Frontend",
			}),
		}}
}

func (r *FrontendReconciler) applyStorage(ctx context.Context, logger logr.Logger, f *webv1.Frontend) error {
	if f.Spec.Backup.NFS.Server == "" || f.Spec.Backup.NFS.Path == "" {
		return nil
	}
	var pv corev1.PersistentVolume
	err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: f.Name}, &pv)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return err
		}
		pv = corev1.PersistentVolume{
			ObjectMeta: defaultObjectMeta(f),
			Spec: corev1.PersistentVolumeSpec{
				Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					NFS: &f.Spec.Backup.NFS,
				},
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
		}

		if err = r.Create(ctx, &pv); err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("create pv"))
	} else { // pv exist
		if pv.Spec.PersistentVolumeSource.NFS.Server != f.Spec.Backup.NFS.Server || pv.Spec.PersistentVolumeSource.NFS.Path != f.Spec.Backup.NFS.Path {
			pv.Spec.PersistentVolumeSource.NFS.Server = f.Spec.Backup.NFS.Server
			pv.Spec.PersistentVolumeSource.NFS.Path = f.Spec.Backup.NFS.Path
			if err = r.Update(ctx, &pv); err != nil {
				return err
			}
			logger.Info(fmt.Sprintf("update pv"))
		}
	}

	// pvc
	var pvc corev1.PersistentVolumeClaim
	err = r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: f.Name}, &pvc)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return err
		}
		pvc = corev1.PersistentVolumeClaim{
			ObjectMeta: defaultObjectMeta(f),
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
				},
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"operator/name": f.Name}},
			},
		}
		if err = r.Create(ctx, &pvc); err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("create pvc"))
	}
	return nil
}
