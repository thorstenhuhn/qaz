package stacks

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/daidokoro/qaz/log"
)

func (s *Stack) terminate() error {
	log.Debug("terminate called for: [%s]", s.Name)
	if !s.StackExists() {
		log.Info("%s: does not exist...", s.Name)
		return nil
	}

	done := make(chan bool)
	svc := cloudformation.New(s.Session, &aws.Config{Credentials: s.creds()})

	params := &cloudformation.DeleteStackInput{
		StackName: aws.String(s.Stackname),
	}

	// create wait handler for tail
	var tailinput = TailServiceInput{
		printed: make(map[string]interface{}),
		stk:     *s,
		command: "DELETE",
	}

	go tailWait(done, &tailinput)

	log.Debug("calling [DeleteStack] with parameters: %s", params)
	if _, err := svc.DeleteStack(params); err != nil {
		done <- true
		return errors.New(fmt.Sprintln("Deleting failed: ", err))
	}

	if err := svc.WaitUntilStackDeleteComplete(&cloudformation.DescribeStacksInput{
		StackName: aws.String(s.Stackname),
	}); err != nil {
		return err
	}

	done <- true
	log.Info("deletion successful: [%s]", s.Stackname)

	return nil
}
