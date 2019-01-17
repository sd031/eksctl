package manager

import (
	"fmt"
	"strings"

	"github.com/kris-nova/logger"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	cfn "github.com/aws/aws-sdk-go/service/cloudformation"

	api "github.com/weaveworks/eksctl/pkg/apis/eksctl.io/v1alpha3"
	"github.com/weaveworks/eksctl/pkg/cfn/builder"
)

func (c *StackCollection) makeClusterStackName() string {
	return "eksctl-" + c.spec.Metadata.Name + "-cluster"
}

// CreateCluster creates the cluster
func (c *StackCollection) CreateCluster(errs chan error, _ interface{}) error {
	name := c.makeClusterStackName()
	logger.Info("creating cluster stack %q", name)
	stack := builder.NewClusterResourceSet(c.provider, c.spec)
	if err := stack.AddAllResources(); err != nil {
		return err
	}

	// Unlike with `CreateNodeGroup`, all tags are already set for the cluster stack
	return c.CreateStack(name, stack, nil, nil, errs)
}

// DescribeClusterStack calls DescribeStacks and filters out cluster stack
func (c *StackCollection) DescribeClusterStack() (*Stack, error) {
	stacks, err := c.DescribeStacks()
	if err != nil {
		return nil, err
	}

	for _, s := range stacks {
		if *s.StackStatus == cfn.StackStatusDeleteComplete {
			continue
		}
		if getClusterName(s) != "" {
			return s, nil
		}
	}
	return nil, nil
}

// DeleteCluster deletes the cluster
func (c *StackCollection) DeleteCluster() error {
	_, err := c.DeleteStack(c.makeClusterStackName())
	return err
}

// WaitDeleteCluster waits till the cluster is deleted
func (c *StackCollection) WaitDeleteCluster() error {
	return c.BlockingWaitDeleteStack(c.makeClusterStackName())
}

// AppendNewClusterStackResource will update cluster
// stack with new resources in append-only way
func (c *StackCollection) AppendNewClusterStackResource() error {
	name := c.makeClusterStackName()

	// NOTE: currently we can only append new resources to the stack,
	// as there are a few limitations:
	// - it must work with VPC that are imported as well as VPC that
	//   is mamaged as part of the stack;
	// - CloudFormation cannot yet upgrade EKS control plane itself;

	currentTemplate, err := c.GetStackTemplate(name)
	if err != nil {
		return errors.Wrapf(err, "error getting stack template %s", name)
	}

	addResources := []string{}

	currentResources := gjson.Get(currentTemplate, resourcesRootPath)
	if !currentResources.IsObject() {
		return fmt.Errorf("unexpected template format of the current stack ")
	}

	logger.Info("creating cluster stack %q", name)
	newStack := builder.NewClusterResourceSet(c.provider, c.spec)
	if err := newStack.AddAllResources(); err != nil {
		return err
	}

	newTemplate, err := newStack.RenderJSON()
	if err != nil {
		return errors.Wrapf(err, "rendering template for %q stack", name)
	}
	logger.Debug("newTemplate = %s", newTemplate)

	newResources := gjson.Get(string(newTemplate), resourcesRootPath)
	if !newResources.IsObject() {
		return fmt.Errorf("unexpected template format of the new version of the stack ")
	}

	logger.Debug("currentTemplate = %s", currentTemplate)

	var iterErr error
	newResources.ForEach(func(resourceKey, resource gjson.Result) bool {
		key := resourceKey.String()
		if currentResources.Get(key).Exists() {
			return true
		}
		addResources = append(addResources, key)
		path := resourcesRootPath + "." + key
		currentTemplate, iterErr = sjson.Set(currentTemplate, path, resource.Value())
		return iterErr == nil
	})
	if iterErr != nil {
		return errors.Wrap(iterErr, "updating stack template")
	}

	if len(addResources) == 0 {
		logger.Success("all resources in cluster stack %q are up-to-date", name)
		return nil
	}

	logger.Debug("currentTemplate = %s", currentTemplate)

	describeUpdate := fmt.Sprintf("updating stack to add new resources: %v", addResources)
	return c.UpdateStack(name, "update-cluster", describeUpdate, []byte(currentTemplate), nil)
}

func getClusterName(s *Stack) string {
	for _, tag := range s.Tags {
		if *tag.Key == api.ClusterNameTag {
			if strings.HasSuffix(*s.StackName, "-cluster") {
				return *tag.Value
			}
		}
	}

	if strings.HasPrefix(*s.StackName, "EKS-") && strings.HasSuffix(*s.StackName, "-ControlPlane") {
		return strings.TrimPrefix("EKS-", strings.TrimSuffix(*s.StackName, "-ControlPlane"))
	}
	return ""
}
