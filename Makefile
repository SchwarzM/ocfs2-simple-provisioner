export GOOS := linux
export GOARCH := amd64

docker: docker/ocfs2-simple-provisioner
	docker build -t schwarzm/ocfs2-simple-provisioner:latest docker/
	docker push schwarzm/ocfs2-simple-provisioner:latest

clean:
	rm docker/ocfs2-simple-provisioner

docker/ocfs2-simple-provisioner:
	go build -a -ldflags '-extldflags "-static"' -o docker/ocfs2-simple-provisioner


#env GOOS=linux GOARCH=amd64 go build -a -ldflags '-extldflags "-static"' -o docker/ocfs2-simple-provisioner .
