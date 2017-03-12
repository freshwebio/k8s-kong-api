FROM busybox
ADD k8s-kong-api /
CMD ["/k8s-kong-api"]
