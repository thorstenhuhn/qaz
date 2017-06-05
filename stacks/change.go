package stacks

import (
	"encoding/json"
	"fmt"
	"qaz/bucket"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"
)

// Change - Manage Cloudformation Change-Sets
func (s *Stack) Change(req, changename string) error {
	svc := cloudformation.New(s.Session, &aws.Config{Credentials: s.creds()})

	switch req {

	case "create":
		// Resolve Deploy-Time functions
		err := s.DeployTimeParser()
		if err != nil {
			return err
		}

		params := &cloudformation.CreateChangeSetInput{
			StackName:     aws.String(s.Stackname),
			ChangeSetName: aws.String(changename),
		}

		Log.Debug(fmt.Sprintf("Updated Template:\n%s", s.Template))

		// If bucket - upload to s3
		var (
			exists bool
			url    string
		)

		if s.Bucket != "" {
			exists, err = bucket.Exists(s.Bucket, s.Session)
			if err != nil {
				Log.Warn(fmt.Sprintf("Received Error when checking if [%s] exists: %s", s.Bucket, err.Error()))
			}

			if !exists {
				Log.Info(fmt.Sprintf(("Creating Bucket [%s]"), s.Bucket))
				if err = bucket.Create(s.Bucket, s.Session); err != nil {
					return err
				}
			}
			t := time.Now()
			tStamp := fmt.Sprintf("%d-%d-%d_%d%d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute())
			url, err = bucket.S3write(s.Bucket, fmt.Sprintf("%s_%s.template", s.Stackname, tStamp), s.Template, s.Session)
			if err != nil {
				return err
			}
			params.TemplateURL = &url
		} else {
			params.TemplateBody = &s.Template
		}

		// If IAM is bening touched, add Capabilities
		if strings.Contains(s.Template, "AWS::IAM") {
			params.Capabilities = []*string{
				aws.String(cloudformation.CapabilityCapabilityIam),
				aws.String(cloudformation.CapabilityCapabilityNamedIam),
			}
		}

		if _, err = svc.CreateChangeSet(params); err != nil {
			return err
		}

		describeParams := &cloudformation.DescribeChangeSetInput{
			StackName:     aws.String(s.Stackname),
			ChangeSetName: aws.String(changename),
		}

		for {
			// Waiting for PENDING state to change
			resp, err := svc.DescribeChangeSet(describeParams)
			if err != nil {
				return err
			}

			Log.Info(fmt.Sprintf("Creating Change-Set: [%s] - %s - %s", changename, Log.ColorMap(*resp.Status), s.Stackname))

			if *resp.Status == "CREATE_COMPLETE" || *resp.Status == "FAILED" {
				break
			}

			time.Sleep(time.Second * 1)
		}

	case "rm":
		params := &cloudformation.DeleteChangeSetInput{
			ChangeSetName: aws.String(changename),
			StackName:     aws.String(s.Stackname),
		}

		if _, err := svc.DeleteChangeSet(params); err != nil {
			return err
		}

		Log.Info(fmt.Sprintf("Change-Set: [%s] deleted", changename))

	case "list":
		params := &cloudformation.ListChangeSetsInput{
			StackName: aws.String(s.Stackname),
		}

		resp, err := svc.ListChangeSets(params)
		if err != nil {
			return err
		}

		for _, i := range resp.Summaries {
			Log.Info(fmt.Sprintf("%s%s - Change-Set: [%s] - Status: [%s]", Log.ColorString("@", "magenta"), i.CreationTime.Format(time.RFC850), *i.ChangeSetName, *i.ExecutionStatus))
		}

	case "execute":
		done := make(chan bool)
		params := &cloudformation.ExecuteChangeSetInput{
			StackName:     aws.String(s.Stackname),
			ChangeSetName: aws.String(changename),
		}

		if _, err := svc.ExecuteChangeSet(params); err != nil {
			return err
		}

		describeStacksInput := &cloudformation.DescribeStacksInput{
			StackName: aws.String(s.Stackname),
		}

		go s.tail("UPDATE", done)

		Log.Debug(fmt.Sprintln("Calling [WaitUntilStackUpdateComplete] with parameters:", describeStacksInput))
		if err := svc.WaitUntilStackUpdateComplete(describeStacksInput); err != nil {
			return err
		}

		done <- true

	case "desc":
		params := &cloudformation.DescribeChangeSetInput{
			ChangeSetName: aws.String(changename),
			StackName:     aws.String(s.Stackname),
		}

		resp, err := svc.DescribeChangeSet(params)
		if err != nil {
			return err
		}

		o, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return err
		}

		fmt.Printf("%s\n", o)
	}

	return nil
}
