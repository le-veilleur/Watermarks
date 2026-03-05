# Génération de mots de passe — Guide complet

## Pourquoi générer des mots de passe aléatoires ?

Un mot de passe inventé par un humain est prévisible — on réutilise des mots,
des dates, des patterns. Un générateur cryptographique puise dans une source
d'entropie (le noyau OS) pour produire des octets **imprévisibles**.

Entropie = mesure de l'imprévisibilité, en bits.
- 8 chars alphanumériques ≈ 48 bits → cassable
- 32 chars aléatoires    ≈ 190 bits → incassable en pratique

---

## OpenSSL

Disponible sur tous les systèmes (Linux, macOS, Windows/WSL).

```bash
# Base64 — caractères : A-Za-z0-9+/=
openssl rand -base64 16   # ~24 caractères
openssl rand -base64 32   # ~44 caractères  ← usage courant
openssl rand -base64 64   # ~88 caractères

# Hexadécimal — caractères : 0-9a-f
# 1 octet = 2 caractères hex, donc longueur exacte = octets × 2
openssl rand -hex 16      # 32 caractères
openssl rand -hex 32      # 64 caractères
openssl rand -hex 48      # 96 caractères
```

### Pourquoi base64 vs hex ?

| Format  | Caractères      | Longueur pour 32 octets | Utilisation typique        |
|---------|-----------------|-------------------------|----------------------------|
| base64  | A-Za-z0-9+/=    | ~44 chars               | Mots de passe, secrets     |
| hex     | 0-9a-f          | 64 chars                | Clés crypto, tokens, UUID  |

> **Attention base64** : les caractères `+`, `/`, `=` posent parfois problème
> dans les URLs ou certains fichiers de config. Utiliser `-hex` dans ce cas,
> ou filtrer avec `tr -d '+/='`.

---

## /dev/urandom — source d'entropie native Linux

`/dev/urandom` est un flux infini d'octets aléatoires généré par le noyau.
OpenSSL l'utilise en interne. On peut l'utiliser directement :

```bash
# Lire 32 octets, garder uniquement les caractères voulus, prendre les 32 premiers
tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 32; echo

# Avec caractères spéciaux
tr -dc 'A-Za-z0-9!@#$%^&*' < /dev/urandom | head -c 32; echo

# Uniquement alphanumérique minuscule
tr -dc 'a-z0-9' < /dev/urandom | head -c 24; echo
```

`tr -dc` = **d**elete **c**omplement — supprime tout ce qui n'est PAS dans la liste.

---

## Python — secrets (module standard)

Disponible sur tout système avec Python 3.6+.
Le module `secrets` est conçu pour la cryptographie, contrairement à `random`.

```bash
# URL-safe base64 (pas de +/=, remplacés par -_)
python3 -c "import secrets; print(secrets.token_urlsafe(32))"

# Hexadécimal
python3 -c "import secrets; print(secrets.token_hex(32))"

# Mot de passe avec alphabet personnalisé
python3 -c "
import secrets, string
alphabet = string.ascii_letters + string.digits + '!@#$'
print(''.join(secrets.choice(alphabet) for _ in range(32)))
"
```

---

## pwgen — mots de passe lisibles par un humain

À installer : `apt install pwgen` / `brew install pwgen`

```bash
pwgen 32 1       # 1 mot de passe de 32 chars (lisible mais moins sécurisé)
pwgen -s 32 1    # -s = secure, totalement aléatoire
pwgen -s 32 5    # génère 5 mots de passe
pwgen -s -y 32 1 # -y = inclut des symboles
```

---

## Tableau comparatif

| Outil                    | Dispo        | Entropie | Caractères spéciaux | Recommandé pour          |
|--------------------------|--------------|----------|---------------------|--------------------------|
| `openssl rand -base64`   | Partout      | Élevée   | Oui (+/=)           | Secrets généraux         |
| `openssl rand -hex`      | Partout      | Élevée   | Non                 | Tokens, clés API         |
| `tr -dc ... /dev/urandom`| Linux/macOS  | Élevée   | Configurable        | Scripts, automatisation  |
| `python3 secrets`        | Partout      | Élevée   | Configurable        | Scripts Python, tokens   |
| `pwgen -s`               | À installer  | Élevée   | Optionnel           | Mots de passe humains    |

---

## Cas pratiques

### Secret pour MinIO / RabbitMQ / base de données

```bash
openssl rand -base64 32
```

### Token API (sans caractères spéciaux)

```bash
openssl rand -hex 32
# ou
python3 -c "import secrets; print(secrets.token_urlsafe(32))"
```

### Clé de chiffrement AES-256 (32 octets exact)

```bash
openssl rand -hex 32   # 256 bits = 32 octets, affiché en 64 chars hex
```

### Générer ET écrire directement dans un fichier secret

```bash
openssl rand -base64 32 | tr -d '\n' > secrets/minio_password.txt
chmod 600 secrets/minio_password.txt
```

> `tr -d '\n'` supprime le retour à la ligne final qu'openssl ajoute —
> évite les bugs subtils où l'app lit `monpassword\n` au lieu de `monpassword`.

---

## Comprendre l'entropie

```
Entropie (bits) = log2(nombre de combinaisons possibles)

Alphabet de 62 chars (A-Za-z0-9), mot de passe de N chars :
  N=8  → log2(62^8)  ≈  48 bits  → cassable avec du matériel
  N=16 → log2(62^16) ≈  95 bits  → très difficile
  N=32 → log2(62^32) ≈ 190 bits  → incassable

openssl rand -base64 32 génère 256 bits d'entropie brute → largement suffisant.
```

**Règle pratique** : au-delà de 128 bits d'entropie, la longueur n'a plus
d'importance pour la sécurité — aucune puissance de calcul actuelle ne peut
brute-forcer ça.

---

## À ne pas faire

```bash
# ❌ date comme seed — prévisible
date | md5sum

# ❌ $RANDOM bash — seulement 15 bits d'entropie
echo $RANDOM

# ❌ random de Python (pas le module secrets)
python3 -c "import random; print(random.randint(0, 999999))"

# ❌ mot de passe en clair dans l'historique bash
export DB_PASS=monpassword123   # visible dans ~/.bash_history
```
