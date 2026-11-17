# hostmon-light

**Docker Image: 6,71 MB**

[Deutsch](README_de.md) | [English](README_en.md)

Ein schlanker Host-Monitoring-Agent für Linux: ein Container, eine überschaubare Konfiguration und eine HTTP-API für die wichtigsten Ressourcenwerte.

hostmon-light soll sich schnell in Betrieb nehmen und leicht auf andere Hosts übertragen lassen – ohne umfangreichen Monitoring-Stack und ohne Konfigurationsmonster. Der Agent liefert Messwerte. Auswertung, Speicherung und Benachrichtigungen übernimmt bei Bedarf ein externes Tool oder Skript.

Die Größenangabe bezieht sich auf das lokale Image der Version 0.1.0 für linux/amd64, nicht auf die komprimierte Downloadgröße.

## Features

- Ein Container ohne zusätzliche Dienste.
- CPU-Auslastung sowie RAM- und Swap-Belegung des Hosts.
- Speicherplatz- und Inode-Belegung konfigurierter Dateisysteme.
- JSON-API mit Bearer-Token-Authentifizierung.
- Token aus einer separaten Datei statt aus der `.env`.
- Konfigurierbares Messintervall und Prüfung auf veraltete Messwerte.
- Integrierter Container-Healthcheck.
- Non-root-Betrieb; standardmäßig UID:GID `65532:65532`.
- Statisch gebaute Go-Anwendung in einem schlanken `scratch`-Image.
- Kein Docker-Socket und keine Docker-API erforderlich.
- Schreibgeschützte Host-Mounts und konfigurierbare Container-Ressourcenlimits.
- Kommentierte Konfiguration und Docker-Logrotation.

## Bewusste Grenzen

Der Agent überwacht Host-Ressourcen, nicht einzelne Docker-Container. Datenbank, Historisierung, Dashboards, Ressourcen-Schwellwerte, Benachrichtigungen und eine allgemeine Plugin-Plattform sind nicht Bestandteil des Projekts.

## Voraussetzungen

- Linux-Host mit Docker Engine und Docker Compose.
- Bisher getestete Plattform: linux/amd64. Die Build-Datei ist darauf festgelegt.
- Ein vorhandenes Quellverzeichnis auf jedem zu überwachenden Dateisystem.
- Bash und OpenSSL für die unten gezeigte Token-Erzeugung.
- Optional `curl` mit `--fail-with-body` und `jq` für die API-Abfrage.
- Ein passendes Image aus einer eigenen Registry oder ein lokaler Build.

Die Beispiele gehen davon aus, dass das Repository bereits heruntergeladen oder geklont wurde und die Befehle im Projektverzeichnis ausgeführt werden.

## Inbetriebnahme

### 1. Konfiguration vorbereiten

Nur bei der Erstinstallation kopieren; eine vorhandene `.env` nicht überschreiben:

```bash
cp .env.example.de .env
```

Alle Parameter sind direkt in der Beispielkonfiguration beschrieben. Für den Betrieb insbesondere prüfen:

| Parameter | Bedeutung |
|---|---|
| `IMAGE_REPOSITORY` | Image-Repository ohne Tag |
| `IMAGE_TAG` | Gewünschte Version |
| `HOST_LAN_IP` | Lokale LAN-Adresse des überwachten Hosts |
| `MONITOR_PORT` | API-Port; Standard `9109` |
| `MONITOR_HOSTNAME` | Hostkennung in der Antwort |
| `MONITOR_DISK_ROOT_SOURCE` | Vorhandenes Quellverzeichnis auf dem zu messenden Dateisystem |
| `MONITOR_TOKEN_HOST_FILE` | Token-Datei; Standard `./secrets/api_token` |

Eine aktive `.env` pro Host genügt. Eine zusätzliche `.env.build` ist nicht erforderlich.

### 2. Image beziehen oder selbst bauen

Beispiel für ein eigenes Repository:

```dotenv
IMAGE_REPOSITORY=registry.example.com/home/hostmon-light
IMAGE_TAG=0.1.0
```

Diese Adresse ist ein Platzhalter, keine öffentliche Downloadquelle. Durch das eigene Repository ersetzen. Zugangsdaten gegebenenfalls über `docker login` hinterlegen, nicht in der `.env`.

Für einen lokalen Build `GO_VERSION` in der `.env` prüfen und ausführen:

```bash
docker compose -f compose.build.yaml config --quiet
docker compose -f compose.build.yaml build hostmon-light
```

Das lokale Image kann auf demselben Host ohne Push verwendet werden. Soll es auf einem anderen Host laufen, optional in die eigene Registry übertragen:

```bash
docker compose -f compose.build.yaml push hostmon-light
```

### 3. Dateisystemquelle vorbereiten

Beispielsweise ein leeres Verzeichnis im Projekt anlegen:

```bash
mkdir -p disk-root
findmnt -T ./disk-root
realpath ./disk-root
```

Den ausgegebenen absoluten Pfad als `MONITOR_DISK_ROOT_SOURCE` eintragen. Für die Messung des Root-Dateisystems muss dieses Verzeichnis tatsächlich auf dem unter `/` eingehängten Dateisystem liegen.

Gemessen wird das zugrunde liegende Dateisystem, nicht die Größe des Verzeichnisses. Ein leeres Verzeichnis vermeidet die Einbindung des vollständigen Host-Verzeichnisbaums.

Zusätzliche Dateisysteme benötigen eigene Bind-Mounts in `compose.yaml` und passende Einträge in `MONITOR_FILESYSTEMS`. Ein zusätzlicher Konfigurationseintrag allein reicht nicht aus. Der API-Pfad wie `/host/disks/root` bezeichnet den Pfad innerhalb des Containers.

### 4. Token-Datei erzeugen

Nur bei der Erstinstallation ausführen. Eine vorhandene Token-Datei nicht ersetzen:

```bash
mkdir -p secrets

(
  set -eu
  set -o noclobber
  umask 077

  openssl rand -hex 32 > secrets/api_token

  sudo chgrp 65532 secrets/api_token
  chmod 640 secrets/api_token
)
```

OpenSSL erzeugt 32 Zufallsbytes, dargestellt als 64 Hexadezimalzeichen. Der Token wird nicht im Terminal ausgegeben. Der Überschreibschutz verhindert das versehentliche Ersetzen einer vorhandenen Datei.

Die Datei bleibt im Besitz des ausführenden Benutzers; Gruppe `65532` erhält Leserechte. Bei abweichender Container-Gruppen-ID die Zahl anpassen. Der Container muss die Datei lesen können.

Die Datei enthält nur den Token, ohne `Bearer ` und ohne Anführungszeichen. In der `.env` steht ausschließlich der Pfad:

```dotenv
MONITOR_TOKEN_HOST_FILE=./secrets/api_token
```

Die lokale `.env` und `secrets/` nicht ins Repository einchecken; entsprechende Einträge in `.gitignore` sicherstellen.

### 5. Starten und prüfen

Zunächst die Betriebskonfiguration validieren:

```bash
docker compose -f compose.yaml config --quiet
```

Bei Verwendung einer Registry das Image herunterladen:

```bash
docker compose -f compose.yaml pull hostmon-light
```

Bei einem lokalen Build auf demselben Host den Pull überspringen. Anschließend:

```bash
docker compose -f compose.yaml up -d --no-build --pull never hostmon-light
docker compose -f compose.yaml ps
docker compose -f compose.yaml logs --tail 50 hostmon-light
```

Die CPU-Erfassung benötigt zunächst zwei Messpunkte. Nach der Aufwärmphase sollte der Container `healthy` melden. Bei längeren Messintervallen die Healthcheck-Startperiode entsprechend berücksichtigen.

## API remote abfragen

Endpunkt: `http://<HOST-IP>:9109/api/v1/metrics`.

Auf einem zugelassenen entfernten Rechner in Bash ausführen. Die Beispieladresse ersetzen:

```bash
(
  set -o pipefail

  HOSTMON_URL='http://192.0.2.10:9109'

  read -rsp 'API token: ' HOSTMON_TOKEN
  printf '\n'

  curl --fail-with-body \
    --silent \
    --show-error \
    --connect-timeout 3 \
    --max-time 10 \
    --header "Authorization: Bearer ${HOSTMON_TOKEN}" \
    "${HOSTMON_URL}/api/v1/metrics" |
    jq .
)
```

Der Token wird verdeckt abgefragt und nicht als eingegebener Tokenwert in der Shell-Historie gespeichert. `jq` formatiert die Antwort; es übernimmt hier keine Schwellwertprüfung.

### Beispielantwort

Erfolgreiche Antwort der Version 0.1.0 mit neutralisiertem Hostnamen. Zeitstempel und Messwerte sind Beispiele:

```json
{
  "schema_version": 1,
  "host": "example-host",
  "sampled_at": "2026-11-16T15:38:13.822603879Z",
  "ready": true,
  "cpu_usage_percent": 3.327451474817052,
  "cpu_sample_seconds": 9.999771396,
  "memory": {
    "total_bytes": 16542408704,
    "available_bytes": 6487289856,
    "used_percent": 60.783885998226154
  },
  "swap": {
    "total_bytes": 0,
    "used_bytes": 0,
    "used_percent": 0
  },
  "filesystems": {
    "root": {
      "path": "/host/disks/root",
      "total_bytes": 503959781376,
      "available_bytes": 365140598784,
      "used_bytes": 113201098752,
      "used_percent": 23.66532111566136,
      "inodes_total": 31252480,
      "inodes_available": 29031968,
      "inodes_used_percent": 7.105074541284404
    }
  },
  "errors": {}
}
```

### Messwerte einordnen

- `schema_version`: Version des Antwortschemas.
- `sampled_at`: Zeitstempel des Mess-Snapshots; `Z` bezeichnet UTC.
- `cpu_usage_percent`: CPU-Auslastung über das unter `cpu_sample_seconds` angegebene Messintervall.
- Byte-Felder enthalten Bytes; Prozentfelder Werte auf einer Skala von 0 bis 100.
- RAM-Belegung: `100 × (1 − available_bytes / total_bytes)`.
- Dateisystembelegung: `used_percent` bezieht sich auf genutzten und verfügbaren Speicher; reservierter Speicher kann erklären, warum `used_bytes + available_bytes` nicht `total_bytes` ergibt.
- `inodes_used_percent`: Belegung der verfügbaren Dateisystem-Dateistrukturen, getrennt vom Speicherplatz.
- Swap mit `total_bytes: 0` zeigt im Beispiel an, dass kein Swap bereitsteht.
- `ready` und `errors` beschreiben die Bereitschaft beziehungsweise Erfassungsfehler, keine Ressourcen-Alarme.

Ein gesunder Agent kann hohe Ressourcenbelegung melden. `healthy` bedeutet, dass die Erfassung bereit und aktuell ist, nicht dass CPU, RAM und Dateisysteme unkritisch sind.

## Netzwerk und Sicherheit

Die Betriebsdatei verwendet Host-Netzwerk und bindet die API an `HOST_LAN_IP:MONITOR_PORT`. Es gibt keine Docker-Portveröffentlichung. Die Bind-Adresse ersetzt keine Firewall.

Den API-Port in der Host-Firewall nur für tatsächlich benötigte Monitoringhosts oder Verwaltungsnetze freigeben und andere Quellen sperren. Bei Änderungen an Hostadresse, Port oder Monitoringhost auch die Regeln überprüfen. Konkrete Firewall-Befehle hängen von der vorhandenen Regelstruktur ab.

Die API verwendet HTTP ohne TLS. Bearer-Authentifizierung verschlüsselt weder Token noch Messwerte. Für nicht vertrauenswürdige Netze einen geschützten Transport wie VPN oder TLS-Reverse-Proxy verwenden. Nicht direkt ungeschützt ins Internet stellen.

Token vertraulich behandeln. Bei einem Wechsel müssen Agent und API-Clients denselben neuen Token verwenden; den Dienst nach dem Wechsel neu starten.

## Monitoringtools und Skripte

Die API lässt sich mit eigenen Skripten oder Werkzeugen nutzen, die HTTP-Anfragen mit Authorization-Headern und eine passende JSON-Auswertung unterstützen.

Ich nutze hostmon-light als Messwertquelle im Zusammenhang mit Uptime Kuma. Die konkrete Einrichtung und Auswertung innerhalb von Uptime Kuma oder anderen Tools ist nicht Bestandteil dieses Repositorys. Das betrifft insbesondere Monitore, Schwellwerte, Prüfintervalle, Wiederholungen, Speicherung und Benachrichtigungen.

## Updates

Gewünschten Release-Tag in `.env` setzen, dann bei Registry-Nutzung:

```bash
docker compose -f compose.yaml config --quiet
docker compose -f compose.yaml pull hostmon-light
docker compose -f compose.yaml up -d --no-build --pull never hostmon-light
docker compose -f compose.yaml ps
```

Bei lokalen Builds zuerst das neue Image mit `compose.build.yaml` bauen und den Pull auslassen. Veröffentlichte Release-Tags nicht mit verändertem Inhalt überschreiben. Eigene `.env` nicht durch eine neue Vorlage ersetzen; neue Parameter bei Bedarf gezielt übernehmen.

## Fehlersuche

| Symptom | Prüfen |
|---|---|
| Keine Verbindung | Hostadresse, Port, Containerstatus, Routing und Firewall |
| HTTP `401` | Token und Authorization-Header |
| HTTP `503` | Aufwärmphase, Messwertalter und Erfassungsfehler |
| Container startet nicht | Logs, Konfiguration, vorhandene Mount-Quellen und Token-Leserechte |
| Falsches Dateisystem | Dateisystem der Mount-Quelle mit `findmnt -T` prüfen |

## Lizenz

MIT-Lizenz; siehe [LICENSE](LICENSE). Lizenzhinweise der verwendeten Abhängigkeiten bleiben davon unberührt.

Copyright (c) 2026 [Jürgen Landt](https://jlandt.de)
