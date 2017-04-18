package main

import (
	"errors"
	"flag"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/external-storage/lib/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
)

const (
	resyncPeriod              = 15 * time.Second
	provisionerName           = "ocfs2-simple-provisioner"
	exponentialBackOffOnError = false
	failedRetryThreshold      = 5
	leasePeriod               = controller.DefaultLeaseDuration
	retryPeriod               = controller.DefaultRetryPeriod
	renewDeadline             = controller.DefaultRenewDeadline
	termLimit                 = controller.DefaultTermLimit
)

type ocfs2SimpleProvisioner struct {
	baseDir     string
	dynDir      string
	modeDynamic bool
}

func NewOcfs2SimpleProvisioner() controller.Provisioner {
	return &ocfs2SimpleProvisioner{
		baseDir: "",
		dynDir:  "",
	}
}

var _ controller.Provisioner = &ocfs2SimpleProvisioner{}

func (p *ocfs2SimpleProvisioner) parseConfig(config map[string]string) {
	p.baseDir = config["basedir"]
	p.dynDir = config["dyndir"]
	if p.baseDir == "" {
		glog.Fatalf("Base directory not set")
	}
	if p.dynDir == "" {
		glog.Fatalf("Dynamic directory not set")
	}
}

func (p *ocfs2SimpleProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {
	p.parseConfig(options.Parameters)
	if _, err := os.Stat(p.baseDir); err != nil {
		return nil, err
	}
	pa := p.baseDir
  pvname := options.PVName
  reclaim := options.PersistentVolumeReclaimPolicy
	modeDynamic := false
	if modestr, ok := options.PVC.Annotations["modeDynamic"]; ok {
		var err error
		modeDynamic, err = strconv.ParseBool(modestr)
		if err != nil {
			return nil, err
		}
	} else {
    return nil, errors.New("Could not find mode annotation")
  }
	if modeDynamic {
		pa = path.Join(p.baseDir, p.dynDir)
		if _, err := os.Stat(pa); err != nil {
			return nil, err
		}
		pa = path.Join(pa, options.PVName)
		if err := os.MkdirAll(pa, 0777); err != nil {
			glog.Infof("creating %v", pa)
			return nil, err
		}
	} else {
    pa = path.Join(p.baseDir, options.PVC.Name)
    glog.Infof("checking if %v exists", pa)
    if _, err := os.Stat(pa); err != nil {
      return nil, errors.New("Could not find static directory it has to exist prior to the claim")
    }
    pvname = options.PVC.Name
    reclaim = v1.PersistentVolumeReclaimRetain
  }
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvname,
			Annotations: map[string]string{
				"modeDynamic": strconv.FormatBool(modeDynamic),
			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: reclaim,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: pa,
				},
			},
		},
	}
	return pv, nil
}

func (p *ocfs2SimpleProvisioner) Delete(volume *v1.PersistentVolume) error {
	modestr, ok := volume.Annotations["modeDynamic"]
	if !ok {
		return errors.New("modeDynamic Annotation not found on PV")
	}
	modeDynamic, err := strconv.ParseBool(modestr)
	if err != nil {
		return err
	}

	//We ignore non dynamic paths
	if modeDynamic {
		pa := path.Join(p.baseDir, p.dynDir)
		pa = path.Join(pa, volume.Name)

		glog.Infof("removing %v", pa)
		if err := os.RemoveAll(pa); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	flag.Parse()
	flag.Set("logtostderr", "true")

	config, err := rest.InClusterConfig()
	if err != nil {
		glog.Fatalf("Failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create client: %v", err)
	}

	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		glog.Fatalf("Failed to get server version: %v", err)
	}

	ocfs2SimpleProvisioner := NewOcfs2SimpleProvisioner()

	pc := controller.NewProvisionController(clientset, resyncPeriod, provisionerName, ocfs2SimpleProvisioner, serverVersion.GitVersion, exponentialBackOffOnError, failedRetryThreshold, leasePeriod, renewDeadline, retryPeriod, termLimit)
	pc.Run(wait.NeverStop)
}
