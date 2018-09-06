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

	ExtConfig extPipelineConfig `yaml:"bosh_release_blobs_upgrader,omitempty"`
}

type extPipelineConfig struct {
	SerialGroups []string `yaml:"serial_groups"`

	TrackFiles []string `yaml:"track_files"`

	ResourceDefaults extPipelineResourceDefaultsConfig `yaml:"resource_defaults"`

	BeforeUpload *atc.PlanConfig `yaml:"before_upload,omitempty"`
	AfterUpload  *atc.PlanConfig `yaml:"after_upload,omitempty"`

	OnFailure *atc.PlanConfig `yaml:"on_failure,omitempty"`
	OnSuccess *atc.PlanConfig `yaml:"on_success,omitempty"`
}

type extPipelineResourceDefaultsConfig struct {
	CheckEvery *string `yaml:"check_every"`
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
		{ // resource/bosh-release-blobs-upgrader-pipeline
			found := false

			for _, resource := range pipeline.Resources {
				if resource.Name == "bosh-release-blobs-upgrader-pipeline" {
					found = true
					break
				}
			}

			if !found {
				pipeline.Resources = append(pipeline.Resources, atc.ResourceConfig{
					Name: "bosh-release-blobs-upgrader-pipeline",
					Type: "git",
					Source: atc.Source{
						"uri": "https://github.com/dpb587/bosh-release-blobs-upgrader-pipeline.git",
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
		blobName := filepath.Base(filepath.Dir(repositoryPath))

		repositoryBytes, err := ioutil.ReadFile(repositoryPath)
		if err != nil {
			panic(err)
		}

		var resourceConfig atc.ResourceConfig
		err = yaml.Unmarshal(repositoryBytes, &resourceConfig)
		if err != nil {
			panic(err)
		}

		resourceConfig.Name = fmt.Sprintf("%s-blob", blobName)

		if pipeline.ExtConfig.ResourceDefaults.CheckEvery != nil {
			resourceConfig.CheckEvery = *pipeline.ExtConfig.ResourceDefaults.CheckEvery
		}

		pipeline.Resources = append(pipeline.Resources, resourceConfig)

		job := atc.JobConfig{
			Name:         fmt.Sprintf("upgrade-%s-blob", blobName),
			SerialGroups: pipeline.ExtConfig.SerialGroups,
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
							Get: "bosh-release-blobs-upgrader-pipeline",
						},
					},
				},
				atc.PlanConfig{
					Task:           "sync-blobs",
					TaskConfigPath: "bosh-release-blobs-upgrader-pipeline/tasks/sync-blobs.yml",
					Params: atc.Params{
						"blob":        blobName,
						"track_files": strings.Join(pipeline.ExtConfig.TrackFiles, " "),
					},
				},
			},
			Success: pipeline.ExtConfig.OnSuccess,
			Failure: pipeline.ExtConfig.OnFailure,
		}

		if pipeline.ExtConfig.OnSuccess != nil {
			j := interpolateBlob(*pipeline.ExtConfig.OnSuccess, blobName)
			job.Success = &j
		}

		if pipeline.ExtConfig.OnFailure != nil {
			j := interpolateBlob(*pipeline.ExtConfig.OnFailure, blobName)
			job.Failure = &j
		}

		if pipeline.ExtConfig.BeforeUpload != nil {
			job.Plan = append(job.Plan, interpolateBlob(*pipeline.ExtConfig.BeforeUpload, blobName))
		}

		if pipeline.ExtConfig.AfterUpload != nil {
			job.Plan = append(
				job.Plan,
				atc.PlanConfig{
					Task:           "upload-blob",
					TaskConfigPath: "bosh-release-blobs-upgrader-pipeline/tasks/upload-blobs.yml",
					Params: atc.Params{
						"release_private_yml": "((release_private_yml))",
						"git_user_email":      "((maintainer_email))",
						"git_user_name":       "((maintainer_name))",
					},
				},
			)

			job.Plan = append(job.Plan, interpolateBlob(*pipeline.ExtConfig.AfterUpload, blobName))
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

func interpolateBlob(plan atc.PlanConfig, blob string) atc.PlanConfig {
	planBytes, err := yaml.Marshal(plan)
	if err != nil {
		panic(err)
	}

	planBytes = []byte(strings.Replace(string(planBytes), "((blob))", blob, -1))

	var planInterpolated atc.PlanConfig

	err = yaml.UnmarshalStrict(planBytes, &planInterpolated)
	if err != nil {
		panic(err)
	}

	return planInterpolated
}
