FROM scratch
ADD k8s-kong-api .
CMD ['/k8s-kong-api']
