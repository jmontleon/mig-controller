package migmigration

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	migapi "github.com/konveyor/mig-controller/pkg/apis/migration/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *Task) runHooks() (bool, error) {
	hook := migapi.MigPlanHook{}
	var client k8sclient.Client
	var err error

	for _, h := range t.PlanResources.MigPlan.Spec.Hooks {
		if (h.Phase + "Hooks") == t.Phase {
			hook = h
		}
	}

	migHook := migapi.MigHook{}
	job := &batchv1.Job{}

	if hook.Reference != nil {
		err = t.Client.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      hook.Reference.Name,
				Namespace: hook.Reference.Namespace,
			},
			&migHook)
		if err != nil {
			log.Trace(err)
			return false, err
		}

		switch migHook.Spec.TargetCluster {
		case "destination":
			client, err = t.getDestinationClient()
			if err != nil {
				log.Trace(err)
				return false, err
			}
		case "source":
			client, err = t.getSourceClient()
			if err != nil {
				log.Trace(err)
				return false, err
			}
		default:
			err := fmt.Errorf("targetCluster must be 'source' or 'destination'. %s unknown", migHook.Spec.TargetCluster)
			log.Trace(err)
			return false, err
		}

		if migHook.Spec.Custom {
			job = t.baseJobTemplate(hook, migHook)
		} else {

			configMap, err := t.configMapTemplate(hook, migHook)
			if err != nil {
				log.Trace(err)
				return false, err
			}

			phaseConfigMap, err := migHook.GetPhaseConfigMap(client, t.Phase)
			if phaseConfigMap == nil && err == nil {

				err = client.Create(context.TODO(), configMap)
				if err != nil {
					log.Trace(err)
					return false, err
				}
			} else if err != nil {
				log.Trace(err)
				return false, err
			}
			job = t.playbookJobTemplate(hook, migHook, configMap.Name)
		}

		runningJob, err := migHook.GetPhaseJob(client, t.Phase)
		if runningJob == nil && err == nil {
			err = client.Create(context.TODO(), job)
			if err != nil {
				log.Trace(err)
				return false, err
			}
			return false, nil
		} else if err != nil {
			log.Trace(err)
			return false, err
		} else if runningJob.Status.Failed >= 5 {
			err := fmt.Errorf("Hook job %s failed.", runningJob.Name)
			log.Trace(err)
			return false, err
		} else if runningJob.Status.Succeeded == 1 {
			return true, nil
		} else {
			return false, nil
		}
	}

	return true, nil
}

func (t *Task) configMapTemplate(hook migapi.MigPlanHook, migHook migapi.MigHook) (*corev1.ConfigMap, error) {

	labels := migHook.GetCorrelationLabels()
	labels["phase"] = t.Phase

	playbookData, err := base64.StdEncoding.DecodeString(migHook.Spec.Playbook)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    hook.ExecutionNamespace,
			GenerateName: strings.ToLower(t.PlanResources.MigPlan.Name + "-" + t.Phase + "-"),
			Labels:       labels,
		},
		BinaryData: map[string][]byte{
			"playbook.yml": []byte(playbookData),
		},
	}, nil
}

func (t *Task) playbookJobTemplate(hook migapi.MigPlanHook, migHook migapi.MigHook, configMap string) *batchv1.Job {
	jobTemplate := t.baseJobTemplate(hook, migHook)

	jobTemplate.Spec.Template.Spec.Containers[0].Command = []string{
		"/bin/entrypoint",
		"ansible-runner",
		"-p",
		"/tmp/playbook/playbook.yml",
		"run",
		"/tmp/runner",
	}

	jobTemplate.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "playbook",
			MountPath: "/tmp/playbook",
		},
	}

	jobTemplate.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "playbook",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMap,
					},
				},
			},
		},
	}

	return jobTemplate
}

func (t *Task) baseJobTemplate(hook migapi.MigPlanHook, migHook migapi.MigHook) *batchv1.Job {
	deadlineSeconds := int64(1800)

	if migHook.Spec.ActiveDeadlineSeconds != 0 {
		deadlineSeconds = migHook.Spec.ActiveDeadlineSeconds
	}

	labels := migHook.GetCorrelationLabels()
	labels["phase"] = t.Phase

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    hook.ExecutionNamespace,
			GenerateName: strings.ToLower(t.PlanResources.MigPlan.Name + "-" + t.Phase + "-"),
			Labels:       labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  strings.ToLower(t.PlanResources.MigPlan.Name + "-" + t.Phase),
							Image: migHook.Spec.Image,
						},
					},
					RestartPolicy:         "OnFailure",
					ServiceAccountName:    hook.ServiceAccount,
					ActiveDeadlineSeconds: &deadlineSeconds,
				},
			},
		},
	}
}
