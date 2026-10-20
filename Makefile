# Distribucion de amerimon (.deb) y utilidades de desarrollo.

REPO_DIR ?= /var/www/html/repo
PORT     ?= 8082

# scripts/clean-rep.sh lee REPO_DIR del entorno.
export REPO_DIR

.PHONY: dist newtags release copy-repo index-repo clean-repo push-repo dev-backend dev-front run

# Los pasos de dist dependen unos de otros: no ejecutar con make -j.
.NOTPARALLEL:

# ---------------------------------------------------------------------------
# Distribucion
# ---------------------------------------------------------------------------

# tag -> release (genera los .deb) -> copiar al repo local -> dejar 3 versiones -> publicar
dist: newtags release copy-repo clean-repo push-repo

# Uso: make dist                -> siguiente patch del tag mas alto; las versiones 0.x se saltan a v1.0.1
#      make dist VERSION=v1.2.0 -> fuerza la version
newtags:
	@if [ -n "$$(git status --porcelain --untracked-files=all)" ]; then \
		echo "ERROR: Hay cambios sin commit; no se creará ni publicará el tag." >&2; \
		git status --short; \
		exit 1; \
	fi
	@NEW_TAG="$(VERSION)"; \
	if [ -z "$$NEW_TAG" ]; then \
		CURRENT_TAG=$$(git tag --list 'v*' --sort=-v:refname | head -n 1); \
		[ -n "$$CURRENT_TAG" ] || CURRENT_TAG=v1.0.0; \
		case "$$CURRENT_TAG" in v0.*) CURRENT_TAG=v1.0.0;; esac; \
		NEW_TAG=$$(echo $$CURRENT_TAG | awk -F. '{print($$1"."$$2"."$$3+1)}'); \
	fi; \
	if ! echo "$$NEW_TAG" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "ERROR: la version debe tener el formato vX.Y.Z (recibido: $$NEW_TAG)" >&2; exit 1; \
	fi; \
	if git rev-parse -q --verify "refs/tags/$$NEW_TAG" >/dev/null; then \
		echo "ERROR: el tag $$NEW_TAG ya existe" >&2; exit 1; \
	fi; \
	echo "Pushing current branch before creating the tag..."; \
	git push && \
	echo "Creating and pushing new tag: $$NEW_TAG" && \
	git tag $$NEW_TAG -m "new tag" && \
	git push --tags

release:
	goreleaser release --clean

copy-repo:
	cp dist/*.deb $(REPO_DIR)/

# Regenera y firma Packages/Release en el repo local.
index-repo:
	cd $(REPO_DIR) && ./update-repo.sh

# Conserva las 3 ultimas versiones de cada paquete y regenera los indices.
clean-repo:
	bash scripts/clean-rep.sh

# Publica en el repo git publico (repo-deb) los .deb vigentes de serverutil y amerimon y los
# indices firmados. Los .deb que clean-repo borro del disco quedan fuera del repo.
push-repo:
	cd $(REPO_DIR) && \
	git ls-files -z 'serverutil_*.deb' 'amerimon_*.deb' | xargs -0 -r git rm --cached -q --ignore-unmatch -- && \
	git add --all -- Packages Packages.gz Release InRelease Release.gpg && \
	find . -maxdepth 1 \( -name 'serverutil_*.deb' -o -name 'amerimon_*.deb' \) -print0 | xargs -0 -r git add -- && \
	{ git diff --staged --quiet || git commit -m "Actualizar paquetes de serverutil y amerimon - $$(git -C $(CURDIR) describe --tags --always)"; } && \
	git push origin main

# ---------------------------------------------------------------------------
# Desarrollo
# ---------------------------------------------------------------------------

# Credenciales AMI tomadas de manager.conf; el secreto JWT es solo de desarrollo.
DEVRUN = AMERIMON_JWT_SECRET=dev-secret \
	AMI_USER=$$(grep -E '^\s*\[.+\]' /etc/asterisk/manager.conf | grep -v '\[general\]' | head -n 1 | tr -d '[] ') \
	AMI_SECRET=$$(grep -E '^\s*secret\s*=' /etc/asterisk/manager.conf | head -n 1 | cut -d= -f2 | tr -d ' ') \
	go run -ldflags "-X main.version=$$(git describe --tags --always)" ./cmd/amerimon -config packaging/amerimon.yaml -addr 127.0.0.1:$(PORT) -db /tmp/amerimon-dev.db

# Backend en :$(PORT); ng serve (:4200) lo usa como proxy para /mon.
dev-backend:
	$(DEVRUN)

dev-front:
	npm start

# Binario unico: compila Angular, lo embebe y lo ejecuta.
run:
	npm run build
	$(DEVRUN)


# Comprobar version
# dpkg-query -W -f='${Version}\n' amerimon
