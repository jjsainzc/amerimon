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
dist: newtags release copy-repo clean-repo push-repo

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

REPO_DIR ?= /var/www/html/repo

# Publica en el repo git publico (repo-deb) los .deb vigentes de serverutil y amerimon y los
# indices firmados. Los .deb que clean-repo borro del disco quedan fuera del repo.
push-repo:
	cd $(REPO_DIR) && \
	git ls-files -z 'serverutil_*.deb' 'amerimon_*.deb' | xargs -0 -r git rm --cached -q --ignore-unmatch -- && \
	git add --all -- Packages Packages.gz Release InRelease Release.gpg && \
	find . -maxdepth 1 \( -name 'serverutil_*.deb' -o -name 'amerimon_*.deb' \) -print0 | xargs -0 -r git add -- && \
	{ git diff --staged --quiet || git commit -m "Actualizar paquetes de serverutil y amerimon - $$(git -C $(CURDIR) describe --tags --always)"; } && \
	git push origin main

# Binario unico: compila Angular, lo embebe y lo ejecuta
run:
	npm run build
	$(DEVRUN)

dist: newtags release copy-repo clean-repo push-repo