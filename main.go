package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/concourse/atc"
	yaml "gopkg.in/yaml.v2"
)

type extPipeline struct {
	atc.Config `yaml:"-,inline"`
	ExtConfig  extPipelineConfig `yaml:"metalink_upgrader_pipeline,omitempty"`
}

type extPipelineConfig struct {
	AfterSyncBlobs   atc.PlanSequence `yaml:"after_sync_blobs"`
	AfterUploadBlobs atc.PlanSequence `yaml:"after_upload_blobs"`
}

type repositoryConfig struct {
	Uri string `yaml:"uri"`
}

func main() {
	var err error
	var releaseDir string

	if len(os.Args) == 3 {
		releaseDir = os.Args[2]
	} else {
		releaseDir, err = os.Getwd()
		if err != nil {
			panic(err)
		}
	}

	pipelineBytes, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	var pipeline extPipeline
	err = yaml.Unmarshal(pipelineBytes, &pipeline)
	if err != nil {
		panic(err)
	}

	repositoryPaths, err := filepath.Glob(filepath.Join(releaseDir, "config", "blobs", "*", "resource.yml"))
	if err != nil {
		panic(err)
	}

	if len(repositoryPaths) > 0 {
		{ // resource/metalink-upgrader-pipeline
			found := false

			for _, resource := range pipeline.Resources {
				if resource.Name == "metalink-upgrader-pipeline" {
					found = true
					break
				}
			}

			if !found {
				pipeline.Resources = append(pipeline.Resources, atc.ResourceConfig{
					Name: "metalink-upgrader-pipeline",
					Type: "git",
					Source: atc.Source{
						"uri": "https://github.com/dpb587/metalink-upgrader-pipeline.git",
					},
				})
			}
		}
	}

	var groupAll = &atc.GroupConfig{Name: "all"}
	var groupBlobs = &atc.GroupConfig{Name: "blobs"}
	var groupBlobsFound bool

	for groupIdx, group := range pipeline.Groups {
		if group.Name == groupAll.Name {
			groupAll = &pipeline.Groups[groupIdx]
		} else if group.Name == groupBlobs.Name {
			groupBlobs = &pipeline.Groups[groupIdx]
			groupBlobsFound = true
		}
	}

	for _, repositoryPath := range repositoryPaths {
		repositoryName := filepath.Base(filepath.Dir(repositoryPath))

		repositoryBytes, err := ioutil.ReadFile(repositoryPath)
		if err != nil {
			panic(err)
		}

		var resourceConfig atc.ResourceConfig
		err = yaml.Unmarshal(repositoryBytes, &resourceConfig)
		if err != nil {
			panic(err)
		}

		resourceConfig.Name = fmt.Sprintf("%s-blob", repositoryName)

		pipeline.Resources = append(pipeline.Resources, resourceConfig)

		job := atc.JobConfig{
			Name: fmt.Sprintf("update-%s-blob", repositoryName),
			Plan: atc.PlanSequence{
				atc.PlanConfig{
					Aggregate: &atc.PlanSequence{
						atc.PlanConfig{
							Get:      "blob",
							Resource: resourceConfig.Name,
							Trigger:  true,
						},
						atc.PlanConfig{
							Get: "repo",
						},
						atc.PlanConfig{
							Get: "metalink-upgrader-pipeline",
						},
					},
				},
				atc.PlanConfig{
					Task:           "sync-blobs",
					TaskConfigPath: "metalink-upgrader-pipeline/tasks/sync-blobs.yml",
					Params: atc.Params{
						"blob": repositoryName,
					},
				},
			},
		}

		job.Plan = append(job.Plan, interpolateBlob(pipeline.ExtConfig.AfterSyncBlobs, repositoryName)...)

		if len(pipeline.ExtConfig.AfterUploadBlobs) > 0 {
			job.Plan = append(
				job.Plan,
				atc.PlanConfig{
					Task:           "upload-blob",
					TaskConfigPath: "metalink-upgrader-pipeline/tasks/upload-blobs.yml",
					Params: atc.Params{
						"release_private_yml": "((release_private_yml))",
						"git_user_email":      "((maintainer_email))",
						"git_user_name":       "((maintainer_name))",
					},
				},
			)

			job.Plan = append(job.Plan, interpolateBlob(pipeline.ExtConfig.AfterUploadBlobs, repositoryName)...)
		}

		pipeline.Jobs = append(pipeline.Jobs, job)
		groupAll.Jobs = append(groupAll.Jobs, job.Name)
		groupBlobs.Jobs = append(groupBlobs.Jobs, job.Name)
	}

	if !groupBlobsFound {
		pipeline.Groups = append(pipeline.Groups, *groupBlobs)
	}

	// remove our config
	pipeline.ExtConfig = extPipelineConfig{}

	newPipelineBytes, err := yaml.Marshal(pipeline)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s\n", newPipelineBytes)
}

func interpolateBlob(plan atc.PlanSequence, blob string) atc.PlanSequence {
	planBytes, err := yaml.Marshal(plan)
	if err != nil {
		panic(err)
	}

	planBytes = []byte(strings.Replace(string(planBytes), "((blob))", blob, -1))

	var planInterpolated atc.PlanSequence

	err = yaml.Unmarshal(planBytes, &planInterpolated)
	if err != nil {
		panic(err)
	}

	return planInterpolated
}
