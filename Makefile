clean:
	rm -rf ./bin/*

release: clean
	@./hack/release.sh

