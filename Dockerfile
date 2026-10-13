# The lab, as an image, because it is run against a real server and the point is that the server
# sees the connections. A static binary in a distroless image: nothing in here needs a shell, and a
# gate with a shell is a gate someone can make pass by hand.
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/lab ./cmd/lab

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lab /lab
ENTRYPOINT ["/lab"]
