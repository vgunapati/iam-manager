package config

import (
	"context"
	"fmt"
	"github.com/keikoproj/iam-manager/pkg/awsapi"
	"github.com/keikoproj/iam-manager/pkg/k8s"
	"github.com/keikoproj/iam-manager/pkg/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"os"
	"strconv"
	"strings"
)

var (
	Props           *Properties
	PolicyARNFormat = "arn:aws:iam::%s:policy/%s"
)

type Properties struct {
	allowedPolicyAction             []string
	restrictedPolicyResources       []string
	restrictedS3Resources           []string
	awsAccountID                    string
	awsMasterRole                   string
	managedPolicies                 []string
	managedPermissionBoundaryPolicy string
	awsRegion                       string
	isWebhookEnabled                string
	maxRolesAllowed                 int
}

func init() {
	log := log.Logger(context.Background(), "internal.config.properties", "init")

	if os.Getenv("LOCAL") != "" {
		err := LoadProperties("LOCAL")
		if err != nil {
			log.Error(err, "failed to load properties for local environment")
			return
		}
		log.Info("Loaded properties in init func for tests")
		return
	}

	k8sClient, err := k8s.NewK8sClient()
	if err != nil {
		log.Error(err, "unable to create new k8s client")
		panic(err)
	}
	res := k8sClient.GetConfigMap(context.Background(), IamManagerNamespaceName, IamManagerConfigMapName)

	// load properties into a global variable
	err = LoadProperties("", res)
	if err != nil {
		log.Error(err, "failed to load properties")
		panic(err)
	}
	log.Info("Loaded properties in init func")
}

func LoadProperties(env string, cm ...*v1.ConfigMap) error {
	log := log.Logger(context.Background(), "internal.config.properties", "LoadProperties")

	// for local testing
	if env != "" {
		Props = &Properties{
			allowedPolicyAction:             strings.Split(os.Getenv("ALLOWED_POLICY_ACTION"), separator),
			restrictedPolicyResources:       strings.Split(os.Getenv("RESTRICTED_POLICY_RESOURCES"), separator),
			restrictedS3Resources:           strings.Split(os.Getenv("RESTRICTED_S3_RESOURCES"), separator),
			awsAccountID:                    os.Getenv("AWS_ACCOUNT_ID"),
			awsMasterRole:                   os.Getenv("AWS_MASTER_ROLE"),
			managedPolicies:                 strings.Split(os.Getenv("MANAGED_POLICIES"), separator),
			managedPermissionBoundaryPolicy: os.Getenv("MANAGED_PERMISSION_BOUNDARY_POLICY"),
			awsRegion:                       os.Getenv("AWS_REGION"),
			isWebhookEnabled:                os.Getenv("ENABLE_WEBHOOK"),
		}
		return nil
	}

	if len(cm) == 0 || cm[0] == nil {
		log.Error(fmt.Errorf("config map cannot be nil"), "config map cannot be nil")
		return fmt.Errorf("config map cannot be nil")
	}

	allowedPolicyAction := strings.Split(cm[0].Data[propertyIamPolicyWhitelist], separator)
	restrictedPolicyResources := strings.Split(cm[0].Data[propertyIamPolicyBlacklist], separator)
	restrictedS3Resources := strings.Split(cm[0].Data[propertyIamPolicyS3Restricted], separator)
	awsMasterRole := cm[0].Data[propertyAwsMasterRole]

	Props = &Properties{
		allowedPolicyAction:       allowedPolicyAction,
		restrictedPolicyResources: restrictedPolicyResources,
		restrictedS3Resources:     restrictedS3Resources,
		awsMasterRole:             awsMasterRole,
	}

	//Defaults
	isWebhook := cm[0].Data[propertWebhookEnabled]
	if isWebhook != "" {
		Props.isWebhookEnabled = isWebhook
	} else {
		Props.isWebhookEnabled = "false"
	}

	awsRegion := cm[0].Data[propertyAwsRegion]
	if awsRegion != "" {
		Props.awsRegion = awsRegion
	} else {
		Props.awsRegion = "us-west-2"
	}

	maxRolesAllowed := cm[0].Data[propertyMaxIamRoles]
	if maxRolesAllowed != "" {
		maxRolesAllowed, err := strconv.Atoi(maxRolesAllowed)
		if err != nil {
			return err
		}
		Props.maxRolesAllowed = maxRolesAllowed
	} else {
		Props.maxRolesAllowed = 1
	}

	awsAccountID := cm[0].Data[propertAWSAccountID]
	// Load AWS account ID
	if Props.awsAccountID == "" && awsAccountID == "" {
		awsAccountID, err := awsapi.NewSTS(Props.awsRegion).GetAccountID(context.Background())
		if err != nil {
			return err
		}
		Props.awsAccountID = awsAccountID
	}

	managedPermissionBoundaryPolicy := cm[0].Data[propertyPermissionBoundary]

	if managedPermissionBoundaryPolicy == "" {
		managedPermissionBoundaryPolicy = "k8s-iam-manager-cluster-permission-boundary"
	}

	Props.managedPermissionBoundaryPolicy = fmt.Sprintf(PolicyARNFormat, awsAccountID, managedPermissionBoundaryPolicy)

	managedPolicies := strings.Split(cm[0].Data[propertyManagedPolicies], separator)
	for i := range managedPolicies {
		if !strings.HasPrefix(managedPolicies[i], "arn:aws:iam::") {
			managedPolicies[i] = fmt.Sprintf(PolicyARNFormat, awsAccountID, managedPolicies[i])
		}
	}
	Props.managedPolicies = managedPolicies

	return nil
}

func (p *Properties) AllowedPolicyAction() []string {
	return p.allowedPolicyAction
}

func (p *Properties) RestrictedPolicyResources() []string {
	return p.restrictedPolicyResources
}

func (p *Properties) RestrictedS3Resources() []string {
	return p.restrictedS3Resources
}

func (p *Properties) ManagedPolicies() []string {
	return p.managedPolicies
}

func (p *Properties) AWSAccountID() string {
	return p.awsAccountID
}

func (p *Properties) AWSMasterRole() string {
	return p.awsMasterRole
}

func (p *Properties) ManagedPermissionBoundaryPolicy() string {
	return p.managedPermissionBoundaryPolicy
}

func (p *Properties) AWSRegion() string {
	return p.awsRegion
}

func (p *Properties) IsWebHookEnabled() bool {
	resp := false
	if p.isWebhookEnabled == "true" {
		resp = true
	}
	return resp
}

func (p *Properties) MaxRolesAllowed() int {
	return p.maxRolesAllowed
}

func RunConfigMapInformer(ctx context.Context) {
	log := log.Logger(context.Background(), "internal.config.properties", "RunConfigMapInformer")
	cmInformer := k8s.GetConfigMapInformer(ctx, IamManagerNamespaceName, IamManagerConfigMapName)
	cmInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: updateProperties,
	},
	)
	log.Info("Starting config map informer")
	cmInformer.Run(ctx.Done())
	log.Info("Cancelling config map informer")
}

func updateProperties(old, new interface{}) {
	log := log.Logger(context.Background(), "internal.config.properties", "updateProperties")
	oldCM := old.(*v1.ConfigMap)
	newCM := new.(*v1.ConfigMap)
	if oldCM.ResourceVersion == newCM.ResourceVersion {
		return
	}
	log.Info("Updating config map", "new revision ", newCM.ResourceVersion)
	err := LoadProperties("", newCM)
	if err != nil {
		log.Error(err, "failed to update config map")
	}
}
