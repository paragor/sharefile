build_local:
	goreleaser release --snapshot --clean

update_default_config:
	rm config_example.yaml || true
	go run main.go --dump-default-config > config_example.yaml
