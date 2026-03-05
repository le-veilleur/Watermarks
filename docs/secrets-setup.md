# Secrets — Setup VPS

Fichiers à créer **une seule fois** manuellement sur le VPS.
Jamais committés (gitignorés), jamais dans les images Docker.

---

## Création

```bash
cd ~/Watermarks

# MinIO
printf '%s' "watermark-minio"          > secrets/minio_user.txt
printf '%s' "$(openssl rand -base64 32)" > secrets/minio_password.txt

# RabbitMQ
PASS=$(openssl rand -base64 32)
printf 'default_user = watermark\ndefault_pass = %s\n' "$PASS" > secrets/rabbitmq.conf

# Permissions — lecture/écriture uniquement pour l'utilisateur courant
chmod 600 secrets/*
```

---

## Vérification

```bash
ls -la secrets/
# -rw------- minio_user.txt
# -rw------- minio_password.txt
# -rw------- rabbitmq.conf

cat secrets/minio_user.txt
cat secrets/minio_password.txt
cat secrets/rabbitmq.conf
```

---

## Structure attendue

```
secrets/
├── minio_user.txt       # login MinIO (ex: watermark-minio)
├── minio_password.txt   # password MinIO (base64 32 octets)
└── rabbitmq.conf        # config RabbitMQ
                         #   default_user = watermark
                         #   default_pass = <generated>
```

---

## Comment Docker les utilise

Le `docker-compose.yml` déclare les secrets en haut :

```yaml
secrets:
  minio_user:
    file: ./secrets/minio_user.txt
  minio_password:
    file: ./secrets/minio_password.txt
```

Docker monte chaque fichier dans `/run/secrets/<nom>` à l'intérieur du conteneur.
L'app Go lit `/run/secrets/minio_user` — la valeur n'est jamais dans les variables d'environnement ni dans l'image.

RabbitMQ est monté directement en volume :

```yaml
volumes:
  - ./secrets/rabbitmq.conf:/etc/rabbitmq/rabbitmq.conf:ro
```

---

## Rotation d'un secret

```bash
# Générer un nouveau password
printf '%s' "$(openssl rand -base64 32)" > secrets/minio_password.txt

# Redémarrer les conteneurs concernés pour prendre en compte la nouvelle valeur
docker compose restart api minio
```

---

## En cas de perte

Si les fichiers sont supprimés ou corrompus, relancer la section **Création** ci-dessus.
Les données MinIO (volume `minio_data`) sont conservées — seul le mot de passe change.
Il faudra alors reconfigurer le mot de passe dans MinIO via la console (`http://VPS:9001`).
