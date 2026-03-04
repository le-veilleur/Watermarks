# CI/CD avec GitHub Actions — Go & React

Guide complet avec exemples précis pour mettre en place des pipelines CI/CD
robustes sur un projet monorepo Go + React.

---

## Table des matières

1. [Concepts fondamentaux](#1-concepts-fondamentaux)
2. [Structure des workflows](#2-structure-des-workflows)
3. [CI — Go](#3-ci--go)
4. [CI — React / Vite](#4-ci--react--vite)
5. [CD — Docker & GHCR](#5-cd--docker--ghcr)
6. [Stratégies de cache](#6-stratégies-de-cache)
7. [Variables, secrets et environnements](#7-variables-secrets-et-environnements)
8. [Patterns avancés](#8-patterns-avancés)
9. [Workflow complet annoté](#9-workflow-complet-annoté)

---

## 1. Concepts fondamentaux

### CI vs CD

| Terme | Définition | Déclenché par |
|---|---|---|
| **CI** (Intégration Continue) | Vérifier que le code fonctionne | Chaque push / PR |
| **CD** (Déploiement Continu) | Livrer automatiquement en prod | Merge sur `main` |

### Anatomie d'un workflow GitHub Actions

```
.github/
└── workflows/
    ├── ci.yml    ← tests, lint, build
    └── cd.yml    ← build Docker, push GHCR, deploy
```

```yaml
name: Mon workflow          # Nom affiché dans l'UI GitHub

on:                         # Déclencheurs
  push:
    branches: [main]

jobs:                       # Unités de travail parallèles
  mon-job:
    runs-on: ubuntu-latest  # Runner GitHub-hébergé
    steps:                  # Étapes séquentielles dans le job
      - uses: actions/checkout@v4
      - run: echo "Hello CI"
```

### Cycle de vie d'un runner

```
Déclencheur (push)
      │
      ▼
GitHub provisionne une VM ubuntu-latest (2 vCPU, 7 Go RAM, 14 Go SSD)
      │
      ▼
Exécution des steps dans l'ordre
      │
      ▼
La VM est détruite — rien ne persiste entre deux runs
```

---

## 2. Structure des workflows

### Déclencheurs courants

```yaml
on:
  # Push sur des branches spécifiques
  push:
    branches: [main, develop]
    paths:                      # Déclencher seulement si ces fichiers changent
      - 'api/**'
      - 'optimizer/**'

  # Pull Request ciblant ces branches
  pull_request:
    branches: [main, develop]

  # Après la fin d'un autre workflow
  workflow_run:
    workflows: [CI]
    branches: [main, develop]
    types: [completed]

  # Déclenchement manuel depuis l'UI GitHub
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environnement cible'
        required: true
        default: 'staging'
        type: choice
        options: [staging, production]

  # Planification (cron)
  schedule:
    - cron: '0 3 * * 1'        # Tous les lundis à 3h du matin
```

### Dépendances entre jobs

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...

  build:
    needs: test                 # build attend que test soit vert
    runs-on: ubuntu-latest
    steps:
      - run: go build ./...

  deploy:
    needs: [test, build]        # deploy attend les deux
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - run: echo "Deploy"
```

---

## 3. CI — Go

### Configuration minimale

```yaml
name: CI Go

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main, develop]

jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod      # Lit la directive `go x.y` dans go.mod
          cache-dependency-path: go.sum

      - run: go vet ./...
      - run: go test ./...
```

### go vet — Analyse statique

`go vet` détecte les erreurs courantes **sans exécuter le code**.

```yaml
- name: Vet
  run: go vet ./...
```

Erreurs détectées par `go vet` :

```go
// ✗ Mauvais format dans Printf
fmt.Printf("%d", "une string")   // go vet: arg "une string" is not int

// ✗ Mutex copié par valeur (race condition potentielle)
func f(mu sync.Mutex) {}          // go vet: passes lock by value

// ✗ Goroutine qui capture une variable de boucle
for _, v := range items {
    go func() { fmt.Println(v) }() // go vet: loop variable v captured by func literal
}
```

### go test — Tests unitaires

```yaml
- name: Tests
  run: go test ./...

# Avec options avancées
- name: Tests complets
  run: |
    go test \
      -v \              # verbose — affiche chaque test
      -count=1 \        # désactive le cache de test
      -timeout 60s \    # timeout global
      ./...
```

### Race detector — Détecter les data races

**Critique pour Go** — les goroutines peuvent accéder aux données en parallèle.

```yaml
- name: Tests avec race detector
  run: go test -race -count=1 ./...
```

Exemple de race détectée :

```go
// Ce code a une data race — deux goroutines écrivent counter sans synchronisation
var counter int

func TestRace(t *testing.T) {
    for i := 0; i < 100; i++ {
        go func() { counter++ }()   // ← race: write
    }
}
// go test -race détecte : DATA RACE on counter
```

### Couverture de code

```yaml
- name: Tests avec couverture
  run: go test -coverprofile=coverage.out ./...

- name: Afficher le rapport
  run: go tool cover -func=coverage.out

# Uploader sur Codecov (optionnel)
- name: Upload coverage
  uses: codecov/codecov-action@v4
  with:
    files: coverage.out
```

Sortie typique :

```
api/main.go:102:    bestFormat          100.0%
api/main.go:113:    detectContentType   100.0%
api/main.go:124:    sendToOptimizer     78.6%
api/main.go:152:    sendResponse        91.7%
total:                                  88.2%
```

### Golangci-lint — Linter avancé

```yaml
- name: golangci-lint
  uses: golangci/golangci-lint-action@v6
  with:
    version: latest
    args: --timeout=5m
```

Règles utiles (`golangci.yml`) :

```yaml
linters:
  enable:
    - errcheck      # vérifie que les erreurs sont gérées
    - gosimple      # suggère des simplifications
    - staticcheck   # analyse statique avancée
    - unused        # variables/fonctions non utilisées
    - govet         # équivalent de go vet
```

### Matrix — Tester sur plusieurs versions Go

```yaml
jobs:
  test:
    strategy:
      matrix:
        go-version: ['1.23', '1.24', '1.25']
        os: [ubuntu-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go-version }}
      - run: go test ./...
```

Résultat : **6 jobs en parallèle** (3 versions × 2 OS).

### Workflow Go complet

```yaml
name: CI — API Go

on:
  push:
    branches: [main, develop]
    paths: ['api/**']
  pull_request:
    branches: [main, develop]

jobs:
  api:
    name: API ${{ matrix.go }}
    runs-on: ubuntu-latest

    strategy:
      fail-fast: false            # continue les autres jobs si l'un échoue
      matrix:
        go: ['1.24', '1.25']

    defaults:
      run:
        working-directory: api

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
          cache-dependency-path: api/go.sum

      - name: Télécharger les dépendances
        run: go mod download

      - name: Vérifier go.mod à jour
        run: |
          go mod tidy
          git diff --exit-code go.mod go.sum   # échoue si go.mod n'est pas à jour

      - name: Vet
        run: go vet ./...

      - name: Tests unitaires
        run: go test -v -count=1 -timeout 60s ./...

      - name: Tests avec race detector
        run: go test -race -count=1 ./...

      - name: Couverture
        run: |
          go test -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out

      - name: Build (vérification compilation)
        run: go build ./...
```

---

## 4. CI — React / Vite

### Configuration minimale

```yaml
name: CI Front

on:
  push:
    branches: [main, develop]

jobs:
  front:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: front
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: npm
          cache-dependency-path: front/package-lock.json

      - run: npm ci
      - run: npm run lint
      - run: npm test
      - run: npm run build
```

### npm ci vs npm install

```yaml
# ✓ CI — installe exactement ce qui est dans package-lock.json
- run: npm ci

# ✗ install — peut mettre à jour les versions → builds non reproductibles
- run: npm install
```

### ESLint — Lint du code React

```yaml
- name: Lint
  run: npm run lint

# Avec sortie formatée pour les PR
- name: Lint avec rapport
  run: npx eslint . --format=github
```

Exemples d'erreurs ESLint attrapées en CI :

```jsx
// eslint-plugin-react-hooks détecte les violations des règles des hooks
function Component() {
  if (condition) {
    useState(0)   // ✗ React Hook appelé conditionnellement
  }
}

// Variables non utilisées
const [value, setValue] = useState(0)
return <div>{value}</div>   // ✗ setValue déclaré mais jamais utilisé
```

### Vitest — Tests unitaires

```yaml
- name: Tests Vitest
  run: npm test       # vitest run dans package.json

# Avec couverture
- name: Tests avec couverture
  run: npx vitest run --coverage
```

Configuration couverture dans `vite.config.js` :

```js
test: {
  environment: 'node',
  coverage: {
    provider: 'v8',
    reporter: ['text', 'lcov'],
    exclude: ['node_modules/', 'dist/'],
  },
}
```

### Vite build — Vérification du bundle

```yaml
- name: Build
  run: npm run build
  env:
    VITE_API_URL: ${{ vars.VITE_API_URL }}   # variable d'environnement Vite

# Analyser la taille du bundle
- name: Analyser le bundle
  run: npx vite-bundle-visualizer
  if: github.event_name == 'pull_request'
```

### Typescript — Vérification des types (si applicable)

```yaml
- name: TypeScript check
  run: npx tsc --noEmit    # vérifie les types sans émettre de fichiers
```

### Workflow React complet

```yaml
name: CI — Front React

on:
  push:
    branches: [main, develop]
    paths: ['front/**']
  pull_request:
    branches: [main, develop]

jobs:
  front:
    name: Front Node ${{ matrix.node }}
    runs-on: ubuntu-latest

    strategy:
      matrix:
        node: ['20', '22']

    defaults:
      run:
        working-directory: front

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: ${{ matrix.node }}
          cache: npm
          cache-dependency-path: front/package-lock.json

      - name: Installer les dépendances
        run: npm ci

      - name: Vérifier package-lock.json à jour
        run: |
          npm install --package-lock-only --ignore-scripts
          git diff --exit-code package-lock.json

      - name: Lint
        run: npm run lint

      - name: Tests unitaires
        run: npm test

      - name: Build production
        run: npm run build
        env:
          VITE_API_URL: http://localhost:4000

      - name: Vérifier la taille du build
        run: du -sh dist/
```

---

## 5. CD — Docker & GHCR

### Connexion à GHCR

```yaml
- name: Login GHCR
  uses: docker/login-action@v3
  with:
    registry: ghcr.io
    username: ${{ github.actor }}          # login GitHub automatique
    password: ${{ secrets.GITHUB_TOKEN }}  # token injecté automatiquement
```

### Tagging des images

```yaml
# Strategy de tags recommandée
- name: Docker metadata
  id: meta
  uses: docker/metadata-action@v5
  with:
    images: ghcr.io/monorg/monimage
    tags: |
      # main → latest
      type=raw,value=latest,enable=${{ github.ref == 'refs/heads/main' }}
      # develop → develop
      type=raw,value=develop,enable=${{ github.ref == 'refs/heads/develop' }}
      # PR → pr-123
      type=ref,event=pr
      # SHA court → abc1234
      type=sha,prefix=,format=short
      # Tag sémantique → v1.2.3 (si git tag)
      type=semver,pattern={{version}}
```

Résultat pour un push sur `main` avec tag `v1.2.3` :

```
ghcr.io/monorg/monimage:latest
ghcr.io/monorg/monimage:abc1234
ghcr.io/monorg/monimage:1.2.3
ghcr.io/monorg/monimage:1.2
ghcr.io/monorg/monimage:1
```

### Build multi-plateforme

```yaml
- name: Set up QEMU           # émulation pour cross-compilation
  uses: docker/setup-qemu-action@v3

- name: Set up Buildx
  uses: docker/setup-buildx-action@v3

- name: Build & push
  uses: docker/build-push-action@v6
  with:
    context: .
    platforms: linux/amd64,linux/arm64   # x86 + Apple Silicon / Raspberry Pi
    push: true
    tags: ${{ steps.meta.outputs.tags }}
    cache-from: type=gha
    cache-to: type=gha,mode=max
```

### Workflow CD complet

```yaml
name: CD

on:
  workflow_run:
    workflows: [CI]
    branches: [main, develop]
    types: [completed]

jobs:
  docker:
    name: Build & push ${{ matrix.service }}
    runs-on: ubuntu-latest
    if: github.event.workflow_run.conclusion == 'success'

    permissions:
      contents: read
      packages: write

    strategy:
      matrix:
        include:
          - service: api
            context: .
            dockerfile: Dockerfile
            build-args: |
              SERVICE_DIR=api
              CMD_PATH=/usr/local/bin/api

          - service: optimizer
            context: .
            dockerfile: Dockerfile
            build-args: |
              SERVICE_DIR=optimizer
              CMD_PATH=/usr/local/bin/optimizer

          - service: front
            context: ./front
            dockerfile: front/Dockerfile
            build-args: ""

    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.workflow_run.head_sha }}

      - name: Login GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Compute tag
        id: tag
        run: |
          BRANCH="${{ github.event.workflow_run.head_branch }}"
          SHA="${{ github.event.workflow_run.head_sha }}"
          if [ "$BRANCH" = "main" ]; then
            echo "stable=latest" >> "$GITHUB_OUTPUT"
          else
            echo "stable=$BRANCH" >> "$GITHUB_OUTPUT"
          fi
          echo "sha=${SHA:0:7}" >> "$GITHUB_OUTPUT"

      - name: Set up Buildx
        uses: docker/setup-buildx-action@v3

      - name: Build & push ${{ matrix.service }}
        uses: docker/build-push-action@v6
        with:
          context: ${{ matrix.context }}
          file: ${{ matrix.dockerfile }}
          target: prod
          build-args: ${{ matrix.build-args }}
          push: true
          tags: |
            ghcr.io/${{ github.repository_owner }}/watermark-${{ matrix.service }}:${{ steps.tag.outputs.stable }}
            ghcr.io/${{ github.repository_owner }}/watermark-${{ matrix.service }}:${{ steps.tag.outputs.sha }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

---

## 6. Stratégies de cache

### Cache Go modules

```yaml
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod
    cache-dependency-path: go.sum   # clé de cache basée sur go.sum
```

`setup-go@v5` gère automatiquement le cache de `$GOPATH/pkg/mod`.

### Cache npm

```yaml
- uses: actions/setup-node@v4
  with:
    node-version: '20'
    cache: npm
    cache-dependency-path: front/package-lock.json
```

### Cache Docker layers (GitHub Actions Cache)

```yaml
- uses: docker/build-push-action@v6
  with:
    cache-from: type=gha          # lire depuis le cache GitHub Actions
    cache-to: type=gha,mode=max   # écrire toutes les layers dans le cache
```

`mode=max` cache toutes les layers intermédiaires, y compris les stages
multi-stage (`build`, `deps`, etc.) — réduit le temps de build de 70-80%.

### Cache manuel avec actions/cache

```yaml
# Exemple : cache des résultats de go build
- name: Cache Go build
  uses: actions/cache@v4
  with:
    path: |
      ~/.cache/go-build
      ~/go/pkg/mod
    key: go-${{ runner.os }}-${{ hashFiles('**/go.sum') }}
    restore-keys: |
      go-${{ runner.os }}-
```

---

## 7. Variables, secrets et environnements

### Hiérarchie des variables

```
Organisation → Repository → Environnement → Workflow → Step
```

### Secrets vs Variables

```yaml
env:
  # Variable publique (visible dans les logs)
  APP_ENV: production

  # Secret (masqué dans les logs avec ***)
  DB_PASSWORD: ${{ secrets.DB_PASSWORD }}

  # Variable de repository (non sensible, configurable sans code)
  API_URL: ${{ vars.API_URL }}
```

### Définir un secret

```
GitHub → Settings → Secrets and variables → Actions → New repository secret
```

```yaml
# Utilisation dans le workflow
- name: Deploy
  env:
    SSH_KEY: ${{ secrets.SSH_PRIVATE_KEY }}
  run: |
    echo "$SSH_KEY" > /tmp/key
    chmod 600 /tmp/key
    ssh -i /tmp/key user@server "docker compose pull && docker compose up -d"
```

### Environnements GitHub (protection rules)

```yaml
jobs:
  deploy-prod:
    environment: production     # requiert une approbation manuelle si configuré
    runs-on: ubuntu-latest
    steps:
      - run: echo "Deploy en production"
```

Dans `Settings → Environments → production` :
- Reviewers requis (approbation humaine)
- Branches autorisées (`main` uniquement)
- Secrets spécifiques à l'environnement

---

## 8. Patterns avancés

### Condition sur la branche ou l'événement

```yaml
steps:
  # Seulement en PR — ne pas déployer sur un push de feature branch
  - name: Commenter la PR
    if: github.event_name == 'pull_request'
    run: echo "Commenter la PR"

  # Seulement sur main
  - name: Deploy prod
    if: github.ref == 'refs/heads/main'
    run: echo "Deploy"

  # Seulement si le job précédent a échoué
  - name: Notifier en cas d'échec
    if: failure()
    run: echo "Pipeline en échec"
```

### Artifacts — Partager des fichiers entre jobs

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: go build -o myapp ./...
      - uses: actions/upload-artifact@v4
        with:
          name: binary
          path: myapp
          retention-days: 7

  test-integration:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          name: binary
      - run: chmod +x myapp && ./myapp --test
```

### path-filter — CI sélective par service

```yaml
on:
  push:
    branches: [main, develop]

jobs:
  detect-changes:
    runs-on: ubuntu-latest
    outputs:
      api: ${{ steps.filter.outputs.api }}
      optimizer: ${{ steps.filter.outputs.optimizer }}
      front: ${{ steps.filter.outputs.front }}
    steps:
      - uses: actions/checkout@v4
      - uses: dorny/paths-filter@v3
        id: filter
        with:
          filters: |
            api:
              - 'api/**'
            optimizer:
              - 'optimizer/**'
            front:
              - 'front/**'

  ci-api:
    needs: detect-changes
    if: needs.detect-changes.outputs.api == 'true'   # run seulement si api/ a changé
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...

  ci-front:
    needs: detect-changes
    if: needs.detect-changes.outputs.front == 'true'
    runs-on: ubuntu-latest
    steps:
      - run: npm test
```

### Notification Slack en cas d'échec

```yaml
- name: Notifier Slack
  if: failure()
  uses: slackapi/slack-github-action@v1
  with:
    payload: |
      {
        "text": "❌ Pipeline échoué sur `${{ github.ref_name }}`",
        "attachments": [{
          "color": "danger",
          "fields": [
            { "title": "Repo", "value": "${{ github.repository }}", "short": true },
            { "title": "Auteur", "value": "${{ github.actor }}", "short": true },
            { "title": "Lien", "value": "${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}" }
          ]
        }]
      }
  env:
    SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}
```

### Déploiement SSH sur VPS

```yaml
- name: Deploy sur VPS
  uses: appleboy/ssh-action@v1
  with:
    host: ${{ secrets.VPS_HOST }}
    username: ${{ secrets.VPS_USER }}
    key: ${{ secrets.SSH_PRIVATE_KEY }}
    script: |
      cd /opt/watermark
      echo "${{ secrets.GHCR_TOKEN }}" | docker login ghcr.io -u ${{ github.actor }} --password-stdin
      docker compose pull
      docker compose up -d --remove-orphans
      docker image prune -f
```

---

## 9. Workflow complet annoté

Le workflow ci-dessous couvre l'intégralité du pipeline : CI, CD, protection
de branche et notification.

### `.github/workflows/ci.yml`

```yaml
name: CI

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main, develop]

jobs:
  # ── API Go ────────────────────────────────────────────────────
  api:
    name: API — Go ${{ matrix.go }}
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ['1.24', '1.25']
    defaults:
      run:
        working-directory: api
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
          cache-dependency-path: api/go.sum

      - run: go mod download
      - run: go vet ./...
      - run: go test -race -count=1 -timeout 60s ./...

  # ── Optimizer Go ──────────────────────────────────────────────
  optimizer:
    name: Optimizer — Go ${{ matrix.go }}
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ['1.24', '1.25']
    defaults:
      run:
        working-directory: optimizer
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
          cache-dependency-path: optimizer/go.sum

      - run: go mod download
      - run: go vet ./...
      - run: go test -race -count=1 -timeout 60s ./...

  # ── Front React ───────────────────────────────────────────────
  front:
    name: Front — Node 20
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: front
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: npm
          cache-dependency-path: front/package-lock.json

      - run: npm ci
      - run: npm run lint
      - run: npm test
      - run: npm run build
        env:
          VITE_API_URL: http://localhost:4000
```

### `.github/workflows/cd.yml`

```yaml
name: CD

on:
  workflow_run:
    workflows: [CI]
    branches: [main, develop]
    types: [completed]

jobs:
  docker:
    name: Docker — ${{ matrix.service }}
    runs-on: ubuntu-latest
    if: github.event.workflow_run.conclusion == 'success'

    permissions:
      contents: read
      packages: write

    strategy:
      matrix:
        include:
          - service: api
            context: .
            dockerfile: Dockerfile
            build-args: |
              SERVICE_DIR=api
              CMD_PATH=/usr/local/bin/api

          - service: optimizer
            context: .
            dockerfile: Dockerfile
            build-args: |
              SERVICE_DIR=optimizer
              CMD_PATH=/usr/local/bin/optimizer

          - service: front
            context: ./front
            dockerfile: front/Dockerfile
            build-args: ""

    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.workflow_run.head_sha }}

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Compute tag
        id: tag
        run: |
          BRANCH="${{ github.event.workflow_run.head_branch }}"
          SHA="${{ github.event.workflow_run.head_sha }}"
          if [ "$BRANCH" = "main" ]; then
            echo "stable=latest" >> "$GITHUB_OUTPUT"
          else
            echo "stable=$BRANCH" >> "$GITHUB_OUTPUT"
          fi
          echo "sha=${SHA:0:7}" >> "$GITHUB_OUTPUT"

      - uses: docker/setup-buildx-action@v3

      - uses: docker/build-push-action@v6
        with:
          context: ${{ matrix.context }}
          file: ${{ matrix.dockerfile }}
          target: prod
          build-args: ${{ matrix.build-args }}
          push: true
          tags: |
            ghcr.io/${{ github.repository_owner }}/watermark-${{ matrix.service }}:${{ steps.tag.outputs.stable }}
            ghcr.io/${{ github.repository_owner }}/watermark-${{ matrix.service }}:${{ steps.tag.outputs.sha }}
          cache-from: type=gha
          cache-to: type=gha,mode=max

  # ── Notification d'échec ──────────────────────────────────────
  notify:
    name: Notifier en cas d'échec
    runs-on: ubuntu-latest
    needs: docker
    if: failure()
    steps:
      - run: |
          echo "Pipeline CD échoué sur ${{ github.event.workflow_run.head_branch }}"
          # Ajouter ici une notification Slack, email, etc.
```

---

## Résumé des durées typiques

| Étape | Go | React |
|---|---|---|
| `go mod download` / `npm ci` (sans cache) | ~20s | ~30s |
| `go mod download` / `npm ci` (avec cache) | ~3s | ~5s |
| `go vet` | ~5s | — |
| `go test -race` | ~30-60s | — |
| `npm run lint` | — | ~10s |
| `npm test` (Vitest) | — | ~5s |
| `npm run build` | — | ~15s |
| Docker build (sans cache) | ~90s | ~60s |
| Docker build (avec cache GHA) | ~15s | ~10s |
| **Total CI** | **~2 min** | **~1 min** |
| **Total CD** | **~3 min** | **~2 min** |
