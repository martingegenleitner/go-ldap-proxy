# Use a builder container to compile and link all assets into a single binary
FROM golang:latest as builder

# Create a working directory and change into it
WORKDIR /go/src/ldap_proxy

# Add the required files for building the final executable
COPY proxy.go go.mod go.sum /go/src/ldap_proxy/

# Run the build
RUN CGO_ENABLED=0 go build -ldflags '-w -s' -o proxy .

##################

# Create an empty container
FROM scratch

# Create a directory for containing all relevant resources
WORKDIR /srv

# Copy the built binary from the builder container into the final image
COPY --from=builder /go/src/ldap_proxy/proxy /srv

# Set a default port for the service to be exposed
EXPOSE 8000

# Start the service by calling the binary
ENTRYPOINT [ "./proxy" ]