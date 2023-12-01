# note the following is required for the makefile help
## COMMAND: DESCRIPTION
## -------: -----------
## help: print each make command with a description
.PHONY: help
help:
	@echo ""
	@(printf ""; sed -n 's/^## //p' oadp.mk) | column -t -s :

## fmt: Run go fmt against code.
fmt:
	go fmt -mod=mod ./...
