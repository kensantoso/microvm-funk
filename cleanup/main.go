// Command cleanup deletes the pty-probe MicroVM images created during the test
// (these are created via the SDK, not Terraform) and terminates any leftover
// MicroVMs.
package main

import (
	"context"
	"flag"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	mv "github.com/aws/aws-sdk-go-v2/service/lambdamicrovms"
	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms/types"
)

func main() {
	region := flag.String("region", "us-east-1", "AWS region")
	prefix := flag.String("prefix", "pty-probe-", "image name prefix to delete")
	flag.Parse()

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(*region))
	if err != nil {
		log.Fatal(err)
	}
	c := mv.NewFromConfig(cfg)

	// Terminate any non-terminated MicroVMs.
	mvs, err := c.ListMicrovms(ctx, &mv.ListMicrovmsInput{})
	if err != nil {
		log.Printf("ListMicrovms: %v", err)
	} else {
		for _, m := range mvs.Items {
			if m.State == types.MicrovmStateTerminated || m.State == types.MicrovmStateTerminating {
				continue
			}
			id := aws.ToString(m.MicrovmId)
			log.Printf("terminating microvm %s (state=%s)", id, m.State)
			if _, err := c.TerminateMicrovm(ctx, &mv.TerminateMicrovmInput{MicrovmIdentifier: aws.String(id)}); err != nil {
				log.Printf("  terminate failed: %v", err)
			}
		}
	}

	// Delete probe images.
	imgs, err := c.ListMicrovmImages(ctx, &mv.ListMicrovmImagesInput{})
	if err != nil {
		log.Fatalf("ListMicrovmImages: %v", err)
	}
	for _, img := range imgs.Items {
		name := aws.ToString(img.Name)
		if !strings.HasPrefix(name, *prefix) {
			continue
		}
		arn := aws.ToString(img.ImageArn)
		log.Printf("deleting image %s (%s)", name, arn)
		if _, err := c.DeleteMicrovmImage(ctx, &mv.DeleteMicrovmImageInput{ImageIdentifier: aws.String(arn)}); err != nil {
			log.Printf("  delete failed: %v", err)
		}
	}
	log.Println("cleanup done")
}
