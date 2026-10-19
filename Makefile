TAG=`git describe --tags`
User = `id -u -n`
DateTime = `date`

newtags:
	@if [ -n "$$(git status --porcelain --untracked-files=all)" ]; then \
		echo "ERROR: Hay cambios sin commit; no se creará ni publicará el tag." >&2; \
		git status --short; \
		exit 1; \
	fi
	@echo "Pushing current branch before creating the tag..."
	@git push
	@# Fallback to v0.0.0 if git describe fails due to no existing tags
	@CURRENT_TAG=$$(git describe --tags 2>/dev/null || echo "v0.0.0"); \
	NEW_TAG=$$(echo $$CURRENT_TAG | awk -F. '{print($$1"."$$2"."$$3+1)}'); \
	echo "Creating and pushing new tag: $$NEW_TAG"; \
	git tag $$NEW_TAG -m "new tag"; \
	git push --tags

release:
	goreleaser release  --clean

copy-repo:
	cp dist/*.deb /var/www/html/repo/

index-repo:
	cd /var/www/html/repo/ && ./update-repo.sh

.PHONY: clean-rep
clean-repo:
	bash scripts/clean-rep.sh

.PHONY: dist dev-backend dev-front run
dist: newtags release copy-repo index-repo

# Desarrollo: backend en :$(PORT) y ng serve (:4200) con proxy /mon -> :8082.
# Toma las credenciales AMI de manager.conf; el secreto JWT es solo de desarrollo.
PORT ?= 8082
DEVRUN = AMERIMON_JWT_SECRET=dev-secret \
	AMI_USER=$$(grep -E '^\s*\[.+\]' /etc/asterisk/manager.conf | grep -v '\[general\]' | head -n 1 | tr -d '[] ') \
	AMI_SECRET=$$(grep -E '^\s*secret\s*=' /etc/asterisk/manager.conf | head -n 1 | cut -d= -f2 | tr -d ' ') \
	go run ./cmd/amerimon -config packaging/amerimon.yaml -addr 127.0.0.1:$(PORT) -db /tmp/amerimon-dev.db

dev-backend:
	$(DEVRUN)

dev-front:
	npm start

# Binario unico: compila Angular, lo embebe y lo ejecuta
run:
	npm run build
	$(DEVRUN)
