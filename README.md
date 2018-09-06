# bosh-release-blobs-upgrader-pipeline

For managing a [BOSH](https://bosh.io) release pipeline which upgrades blobs from external resources.

Declare your dependencies as Concourse resources to track versions and ensure they can be downloaded; then configure tests to run after upgrading a blob and before pushing or submitting a pull request with the upgrades.


## Usage

The `config/blobs` directory of a release repository is used to record and track sourcing information of dependencies. Each subdirectory defines a dependency for tracking an external, versioned resource through a Concourse [resource](https://concourse-ci.org/resources.html) definition in the `resource.yml` file. For example, a dependency on [PHP 7](https://php.net/) might have a file `config/blobs/php7/resource.yml` which looks like...

```yaml
type: dynamic-metalink
source:
  version_check: |
    curl -s https://golang.org/dl/?mode=json | jq -r '.[].version[2:]'
  metalink_get: |
    curl -s https://golang.org/dl/?mode=json | jq '
      map(select(.version[2:] == env.version)) | map({
        "files": (.files | map({
          "name": .filename,
          "size": .size,
          "urls": [ { "url": "https://dl.google.com/go/\(.filename)" } ],
          "hashes": [ { "type": "sha-256", "hash": .sha256 } ] } ) ) } )[]'
  include_files:
  - go*.linux-amd64.tar.gz
```

Each dependency will be placed into its own directory under `blobs` (since a dependency may have multiple files). When a new version is available, all existing files in its blobs directory will be removed and replaced with the new dependency blobs. Additionally, resource metadata can be committed as well to help document the provenance of the blob. In this example, `config/blobs.yml` would eventually have an entry for `php7/php-7.2.8.tar.gz` and `config/blobs/php7/metalink.meta4` would document where exactly it was downloaded from.

A pipeline defines the basic configuration (e.g. `ci/pipelines/upgrader.yml`) which includes, at a minimum, a `repo` resource which references the release repository and some additional upgrade steps in a `bosh_release_blobs_upgrader` section.

```yaml
bosh_release_blobs_upgrader:
  # run integration tests with a dev release of the new blob to make sure it works
  before_upload:
    do:
    - task: create-dev-release
      file: repo/ci/tasks/create-dev-release/task.yml
    - task: integration-test
      file: repo/ci/tasks/integration-test/task.yml
      privileged: true
  # push the new blobs to the repo
  after_upload:
    put: repo
    params:
      repository: repo
  # notify slack/email to ensure someone manually figures out upgrading
  on_failure:
    task: notify-upgrade-failure
    file: repo/ci/tasks/notify-upgrade-failure/task.yml
  # track details about where the blobs came from
  track_files:
  - .resource/metalink.meta4
resources:
# the bosh release repository
- name: repo
  type: git
  source:
    uri: git@github.com:dpb587/ssoca-bosh-release.git
    branch: master
    private_key: ((repo_private_key))
resource_types:
# used by individual resource.yml definitions
- name: dynamic-metalink
  type: docker-image
  source:
    repository: dpb587/dynamic-metalink-resource
```

Once the blobs and base pipeline have been configured, `bosh-release-blobs-upgrader-pipeline` can be used to generate the pipeline for passing to `fly`. The added blob upgrade jobs will require several variables to be set: `release_private_yml` (for uploading blobs to the blobstore) and `maintainer_email`, `maintainer_name` (for the `git` commit).

```bash
$ fly set-pipeline -p my-release:upgrader \
  -c <( bosh-release-blobs-upgrader-pipeline ci/pipelines/upgrader.yml ) \
  -l <( lpass show 'pipeline=my-release:upgrader' )
```

The blob jobs automatically trigger whenever a new version is available. When syncing blobs, old blobs in the directory are removed, new blobs are added (not yet uploaded), and a commit message referencing the specific blob and new version are prepared. When uploading blobs, `config/blobs.yml` is updated with the new blobstore references, and any other already-staged files are committed. After uploading blobs, the repository should be pushed.


## Examples

A couple repositories using this to manage some upstream dependencies...

* [dpb587/openvpn-bosh-release](https://github.com/dpb587/openvpn-bosh-release/blob/master/ci/pipelines/upgrader.yml)
* [dpb587/ssoca-bosh-release](https://github.com/dpb587/ssoca-bosh-release)
