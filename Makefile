build:
	docker build --platform=linux/amd64,linux/arm64,darwin/arm64,windows/amd64 -t natk64/pancake:latest .
