# metalink-upgrader-pipeline

For managing a [BOSH](https://bosh.io) release pipeline which upgrades blobs from [metalink repositories](https://github.com/dpb587/metalink/tree/master/repository#metalink-repository).


## Usage

A `config/blobs/{blobname}/repository.yml` defines the metalink repository where the blob versions are mirrored.

    uri: git+https://github.com/dpb587/upstream-blob-mirror.git//repository/icu4c
    version: ^58

A pipeline defines the basic configuration for an upgrader pipeline (`ci/pipelines/upgrader.yml`) which defines, at a minimum, a `repo` resource which references the release repository. After a blob upgrade is triggered, the `config.after_upload_blobs` setting configures what happens after the blobs have been synced.

    config:
      after_sync_blobs: # run integration tests with a dev release of the new blob
      - task: create-dev-release
        file: repo/ci/tasks/create-dev-release/task.yml
      - task: integration-test
        file: repo/ci/tasks/integration-test/task.yml
        privileged: true
      after_upload_blobs: # push the new blobs to the repo
      - put: repo
        params:
          repository: repo
    resources:
    - name: repo # the bosh release repository
      type: git
      source:
        uri: git@github.com:dpb587/openvpn-bosh-release.git
        branch: master
        private_key: ((repo_private_key))

Once the blobs and base pipeline have been configured, `metalink-upgrader-pipeline` can be used to generate a pipeline. The generated pipeline will have a new job to handle updates to the upstream blobs. By configuring steps for `after_upload_blobs`, a `bosh upload-blobs` step will first be executed. The job will require several variables to be set: `release_private_yml` (for uploading blobs to the blobstore) and `maintainer_email`, `maintainer_name` (for the `git` commit).

    fly set-pipeline -p openvpn:upgrader \
      -c <( metalink-upgrader-pipeline ci/pipelines/upgrader.yml ) \
      -l <( lpass show 'pipeline=openvpn:upgrader' )

The blob jobs automatically trigger whenever a new version is available. When syncing blobs, old blobs in the directory are removed, new blobs are added (not yet uploaded), and a copy of the origin metalink is staged to `config/blobs/{blobname}/metalink.meta4`. When uploading blobs, `config/blobs.yml` is updated with the new blobstore references, and any other already-staged files are committed. After uploading blobs, the repository should be pushed.


## Examples

A couple public repositories using this to manage some upstream dependencies.

* [dpb587/openvpn-bosh-release](https://github.com/dpb587/openvpn-bosh-release)
* [dpb587/ssoca-bosh-release](https://github.com/dpb587/ssoca-bosh-release)
