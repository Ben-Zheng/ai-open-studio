package controllers

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	workspacev1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/apis/operator/api/v1"
)

func NewSecret(app *workspacev1.AWorkspace) *corev1.Secret {
	userDataTemp := `#cloud-config
ssh_pwauth: true
disable_root: false
ssh_authorized_keys:
- ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCh1O4WpVIYYT/o6tMO039o7MvmJOJarLx5tY4kd0nPnDx4x+LdOKOx7HQjXjx7gah1y42cnweROmXAc8MVuAwQ7FPn+aqvHfUrzE07bdtt4d0LoFz5gkSjE+YYYgw7OWS0ZHVPqZpZELzE//bAwtfSxzNLESXN10tnHWW9qEjpMUprd+b8j5p2R5hHsIv2PVrPhs91WTGF6rkWL93GLbXcEfLO3FfjVEeVC4QAzWTjKWoLB1IGkkUvSW/yrhfp0AjZpHr7tIqeBIi68rI1pgZWSvIoZ9qcswnI5P+a+8AAkfRTl1vgFP+QhqMXY7v3gaE3MMZI8g7r/TUcBkLrtC7r ais@megvii.com
bootcmd:
- [ bash, -c, echo "%s" > /usr/local/aiservice/env ]
- [ bash, -c, sed -i -e "s/c.NotebookApp.port = 80/c.NotebookApp.port = 8888/g" /etc/jupyter/jupyter_notebook_config.py]
- [ bash, -c, "export LIBRARY_MIRROR=https://mirrors.tuna.tsinghua.edu.cn/ubuntu/ DEBIAN_FRONTEND=noninteractive LANG=C.UTF-8 LC_ALL=C.UTF-8 LD_LIBRARY_PATH=/usr/lib/x86_64-linux-gnu/:$LD_LIBRARY_PATH PYTHONPATH=/home/aiservice/workspace/.py/lib/python3.7/site-packages PATH=~/.local/bin:/home/aiservice/workspace/.py/bin:/home/aiservice/.miniconda/bin:$PATH; export ` + "`cat /usr/local/aiservice/env`" + `;conda run -n ais python /usr/local/aiservice/init.py; ( conda run -n ais python /usr/local/aiservice/resource.py & ); ( /usr/local/vscode/bin/code-server --bind-addr=0.0.0.0:9826 --auth=none --disable-telemetry --user-data-dir=/opt/vscode/ & ); (conda run -n ais jupyter lab --ip=0.0.0.0 --config=/etc/jupyter/jupyter_notebook_config.py --allow-root &)" ]
device_aliases:
  sda: /dev/sda
disk_setup:
  sda:
    table_type: gpt
    layout: true
    overwrite: true
fs_setup:
- label: data
  filesystem: ext4
  device: sda
  cmd: mkfs -t %%(filesystem)s -L %%(label)s %%(device)s
mounts:
- ["/dev/sda", "/home/aiservice/workspace"]
`
	envStr := ""
	for _, env := range app.Spec.Envs {
		envStr = fmt.Sprintf("%s %s=%s", envStr, env.Name, env.Value)
	}
	userData := fmt.Sprintf(userDataTemp, envStr)

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        app.Name,
			Namespace:   app.Namespace,
			Labels:      app.Spec.Labels,
			Annotations: app.Spec.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, schema.GroupVersionKind{
					Group:   app.GroupVersionKind().Group,
					Version: app.GroupVersionKind().Version,
					Kind:    "AWorkspace",
				}),
			},
		},
		StringData: map[string]string{
			"userData": userData,
		},
	}
}

func UpdateSecret(osct *corev1.Secret, app *workspacev1.AWorkspace) *corev1.Secret {
	nsct := NewSecret(app)

	osct.Labels = nsct.Labels
	osct.Annotations = nsct.Annotations
	osct.StringData = nsct.StringData

	return osct
}

func NewVM(app *workspacev1.AWorkspace) *v1.VirtualMachine {
	labels := map[string]string{}
	for key, value := range app.Spec.Labels {
		labels[key] = value
	}

	labels[aisConst.AISWSInstanceID] = app.Spec.Annotations[aisConst.AISWSInstanceID]
	labels[aisConst.AISWSInstanceToken] = app.Spec.Annotations[aisConst.AISWSInstanceToken]

	running := true
	readOnly := true
	storageClassName := "local-path-kubevirt"
	imageURL := fmt.Sprintf("docker://%s", app.Spec.Image)
	dvName := fmt.Sprintf("%s-dv", app.Name)
	networkData := `ethernets:
  enp1s0:
    dhcp4: true
version: 2`

	gpus := []v1.GPU{}
	if gpuNum, ok := app.Spec.Resources.Limits[aisConst.NVIDIAGPUResourceName]; ok {
		for i := int64(0); i < gpuNum.Value(); i++ {
			gpus = append(gpus, v1.GPU{
				Name:       fmt.Sprintf("gpu%d", i),
				DeviceName: app.Spec.Annotations[aisConst.AISWSGPUDeviceName],
			})
		}
	}

	limits := corev1.ResourceList{}
	for k, v := range app.Spec.Resources.Limits {
		if k != aisConst.NVIDIAGPUResourceName {
			limits[k] = v
		}
	}

	// 校正 vm 使用的内存，去掉 Overhead 部分
	memory, cpu := limits["memory"], limits["cpu"]
	memory.Sub(resource.MustParse("232Mi"))
	memory.Sub(resource.MustParse(fmt.Sprintf("%dMi", cpu.Value()*8)))
	if len(gpus) > 0 {
		memory.Sub(resource.MustParse("1024Mi"))
	}
	memory.Set(memory.Value()*512/513 - 1)
	limits["memory"] = memory

	return &v1.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubevirt.io/v1",
			Kind:       "VirtualMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        app.Name,
			Namespace:   app.Namespace,
			Labels:      labels,
			Annotations: app.Spec.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(app, schema.GroupVersionKind{
					Group:   app.GroupVersionKind().Group,
					Version: app.GroupVersionKind().Version,
					Kind:    "AWorkspace",
				}),
			},
		},
		Spec: v1.VirtualMachineSpec{
			Running: &running,
			Template: &v1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: app.Spec.Annotations,
				},
				Spec: v1.VirtualMachineInstanceSpec{
					Affinity: app.Spec.Affinity,
					NodeSelector: map[string]string{
						aisConst.AISWSRuntime: aisConst.WSRuntimeKubevirt,
					},
					Domain: v1.DomainSpec{
						Devices: v1.Devices{
							Disks: []v1.Disk{
								{
									Name: "dvdisk0",
									DiskDevice: v1.DiskDevice{
										Disk: &v1.DiskTarget{
											Bus: v1.DiskBusVirtio,
										},
									},
								},
								{
									Name:       "dvdisk1",
									DiskDevice: v1.DiskDevice{},
								},
								{
									Name: "cloudinitdisk",
									DiskDevice: v1.DiskDevice{
										CDRom: &v1.CDRomTarget{
											Bus:      v1.DiskBusSATA,
											ReadOnly: &readOnly,
										},
									},
								},
							},
							GPUs: gpus,
						},
						Resources: v1.ResourceRequirements{
							Limits: limits,
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "dvdisk0",
							VolumeSource: v1.VolumeSource{
								DataVolume: &v1.DataVolumeSource{
									Name: dvName,
								},
							},
						},
						{
							Name: "dvdisk1",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: app.Name,
									},
								},
							},
						},
						{
							Name: "cloudinitdisk",
							VolumeSource: v1.VolumeSource{
								CloudInitNoCloud: &v1.CloudInitNoCloudSource{
									NetworkData: networkData,
									// UserData:    userData,
									UserDataSecretRef: &corev1.LocalObjectReference{
										Name: app.Name,
									},
								},
							},
						},
					},
				},
			},
			DataVolumeTemplates: []v1.DataVolumeTemplateSpec{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:        dvName,
						Labels:      labels,
						Annotations: app.Spec.Annotations,
					},
					Spec: v1beta1.DataVolumeSpec{
						PVC: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: &storageClassName,
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: app.Spec.Resources.Limits["ephemeral-storage"],
								},
							},
						},
						Source: &v1beta1.DataVolumeSource{
							Registry: &v1beta1.DataVolumeSourceRegistry{
								URL: &imageURL,
							},
						},
					},
				},
			},
		},
	}
}

func UpdateVM(oldVM *v1.VirtualMachine, app *workspacev1.AWorkspace) *v1.VirtualMachine {
	newVM := NewVM(app)

	oldVM.Labels = newVM.Labels
	oldVM.Annotations = newVM.Annotations
	oldVM.Spec.Running = newVM.Spec.Running
	oldVM.Spec.Template.ObjectMeta.Labels = newVM.Spec.Template.ObjectMeta.Labels
	oldVM.Spec.Template.ObjectMeta.Annotations = newVM.Spec.Template.ObjectMeta.Annotations
	oldVM.Spec.Template.Spec = newVM.Spec.Template.Spec
	if len(oldVM.Spec.DataVolumeTemplates) > 0 {
		oldVM.Spec.DataVolumeTemplates[0].Labels = newVM.Spec.DataVolumeTemplates[0].Labels
		oldVM.Spec.DataVolumeTemplates[0].Annotations = newVM.Spec.DataVolumeTemplates[0].Annotations
		oldVM.Spec.DataVolumeTemplates[0].Spec = newVM.Spec.DataVolumeTemplates[0].Spec
	} else {
		oldVM.Spec.DataVolumeTemplates = newVM.Spec.DataVolumeTemplates
	}

	return oldVM
}
