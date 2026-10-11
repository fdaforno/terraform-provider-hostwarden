.PHONY: build test vet docs

build: vet test ## vet, test and build the provider binary in this folder
	go build -o terraform-provider-hostwarden .

vet:
	go vet ./...

test:
	go test ./...

docs: ## regenerate docs/ (the pages shown on the Terraform Registry) from the schema
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.21.0 generate --provider-name hostwarden
