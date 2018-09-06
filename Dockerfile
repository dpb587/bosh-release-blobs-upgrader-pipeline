FROM alpine:3.6
RUN apk --no-cache add bash curl git
RUN \
  curl -o /usr/local/bin/bosh http://s3.amazonaws.com/bosh-cli-artifacts/bosh-cli-2.0.45-linux-amd64 \
  && echo 'bf04be72daa7da0c9bbeda16fda7fc7b2b8af51e  /usr/local/bin/bosh' | sha1sum -c \
  && chmod +x /usr/local/bin/bosh
ADD bin /opt/bosh-release-blobs-upgrader-pipeline/bin
