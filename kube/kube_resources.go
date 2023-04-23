package kube

import (
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

type KubeMsg int 
const(
	CREATE_SERVICE KubeMsg = iota
	DELETE_SERVICE
	CREATE_DEPLOYMENT
	DELETE_DEPLOYMENT
)

type KubeCmd struct{
	Type KubeMsg
	ObjectDeployment KubeDeployment 
	ObjectService	KubeNodePort
}

type KubeNodePort struct{
	Name string
	Selector []KubeLable
	Port KubePort
}

type KubeLable struct{
	Key string
	Value string
}

type KubeImage struct{
	Name string
	ImageName string
	Ports KubePort
}

type KubePort struct{
	Name string
	ContainerPort int32
	TargetPort int32
	NodePort int32
}

type KubeDeployment struct{
	Name string
	Replicas int32
	Selector []KubeLable
	Labels []KubeLable
	Image KubeImage
	Ports KubePort
}

func CreateDeploymentObject(metaName string, numReplicas int32, selectKey string, selectValue string, labelKey string, 
	labelValue string,cntrName string, cntrImage string,  cntrPort int32, targetPort int32, portName string) (error, *KubeDeployment){
	obj := KubeDeployment{
		Name: metaName,
		Replicas: numReplicas,
		Selector: []KubeLable{
			{Key: selectKey, Value: selectValue},
		},
		Labels: []KubeLable{
			{Key: labelKey, Value: labelValue},
		},
		Image: KubeImage{
			Name: cntrName,
			ImageName: cntrImage,
			Ports: KubePort{
				Name: portName,
				ContainerPort: cntrPort,
			},
		},
	}
	return nil, &obj


}
func CreateNodePortObject(metaName string, portName string, cntrPort int32, targetPort int32, nodePort int32, 
	selectKey string, selectValue string) (error, *KubeNodePort){
	if cntrPort == 0 || targetPort == 0 || nodePort == 0 {
		return fmt.Errorf("MISSING SERVICE NODEPORT PORT DEFINITION"), &KubeNodePort{}
	}
	if selectKey == "" || selectValue == "" {
		return fmt.Errorf("MISSING SELECTOR VALUE - NO HEADLESS SERVICE DEFINITION"), &KubeNodePort{}
	}
	obj := KubeNodePort{
		Name: metaName,
		Port: KubePort{
			Name: portName,
			ContainerPort: cntrPort,
			TargetPort: targetPort,
			NodePort: nodePort,
		},
		Selector: []KubeLable{
			{Key: selectKey, Value: selectValue},
		},
	}
	return nil, &obj
}

func tupleToDict(labels []KubeLable) map[string]string{
	dict := make(map[string]string)
	for _, tuple := range labels{
		if (tuple.Key == "" || tuple.Value == ""){continue}
		dict[tuple.Key]=tuple.Value
	}
	return dict
}

func CreateNodePort(clstr *kubernetes.Clientset, object KubeNodePort) error{
	nodeportObject := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: object.Name,
		},
		Spec: apiv1.ServiceSpec{
			Ports: []apiv1.ServicePort{
				{
					Name:       object.Port.Name,
					Port:       object.Port.ContainerPort,
					TargetPort: intstr.FromInt(int(object.Port.TargetPort)),
					NodePort: object.Port.NodePort,
				},
			},
			Selector: tupleToDict(object.Selector),
			Type: apiv1.ServiceTypeNodePort,
		},
	}
	_, err := clstr.CoreV1().Services("default").Create(context.TODO(), nodeportObject, metav1.CreateOptions{})
	if err != nil{
		return err
	}
	return nil 
}

// Created through template client-go official repo: https://github.com/kubernetes/client-go/blob/master/examples/create-update-delete-deployment/main.go
func CreateDeployment(clstr *kubernetes.Clientset, object KubeDeployment) error{
	deploymentObject := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: object.Name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &object.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: tupleToDict(object.Selector),
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: tupleToDict(object.Labels),
				},
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Name:  object.Image.Name,
							Image: object.Image.ImageName,
							Ports: []apiv1.ContainerPort{
								{
									Name:          object.Ports.Name,
									Protocol:      apiv1.ProtocolTCP,
									ContainerPort: object.Ports.ContainerPort,
								},
							},
						},
					},
				},
			},
		},
	}
	_, err := clstr.AppsV1().Deployments(apiv1.NamespaceDefault).Create(context.TODO(), deploymentObject, metav1.CreateOptions{})
	if err != nil{
		return err 
	}
	return nil 
}

