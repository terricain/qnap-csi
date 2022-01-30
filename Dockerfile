FROM golang:1.17.6-alpine3.15 AS build

WORKDIR /usr/local/go/src/cmd/qnap-csi-plugin

COPY go.mod go.sum /usr/local/go/src/cmd/qnap-csi-plugin/

RUN go mod download

ADD cmd/ /usr/local/go/src/cmd/qnap-csi-plugin/cmd/
ADD driver/ /usr/local/go/src/cmd/qnap-csi-plugin/driver/
ADD qnap/ /usr/local/go/src/cmd/qnap-csi-plugin/qnap/

RUN ls -lah

RUN go build -o /plugin cmd/qnap-csi-plugin/main.go
RUN go build -o /iscsiadm cmd/iscsiadm/main.go

FROM alpine:3.15.0 AS release
RUN apk update && \
    apk add lsblk e2fsprogs xfsprogs util-linux-misc
# lsblk
# e2fsprogs -> mkfs.ext3, mkfs.ext4, fsck.ext3, fsck.ext4
# xfsprogs -> mkfs.xfs, fsck.xfs
# util-linux-misc -> mount
COPY --from=build /plugin /plugin
COPY --from=build /iscsiadm /sbin/iscsiadm

ENTRYPOINT ["/plugin"]

