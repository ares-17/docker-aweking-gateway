# GEMINI.md – Contesto di sviluppo: Docker Awakening Gateway

Benvenuto! Questo documento descrive il progetto **Docker Awakening Gateway**, un container Docker ultraleggero che funge da reverse proxy on-demand per container Docker, con una pagina di attesa minimal mentre il container di destinazione si avvia.

## 📋 Panoramica del progetto

**Nome provvisorio**: Docker Awakening Gateway  
**Obiettivo**: Creare un gateway HTTP che, quando riceve una richiesta per un container Docker **spento**, mostra una UI di attesa (statica, leggera), avvia il container e poi reindirizza automaticamente l'utente al servizio. Se il container è già **acceso**, la richiesta viene passata direttamente senza mostrare alcuna UI intermedia.

**Perché**: In ambienti con risorse limitate (edge, home lab, cluster economici) è utile tenere spenti i container poco utilizzati e riattivarli solo quando necessario, senza richiedere all'utente di avviarli manualmente.

## 🎯 Requisiti chiave

- **Leggerezza**: Il container del gateway deve consumare pochissima RAM/CPU (ideale < 10 MB, possibilmente in Go o Rust).
- **Sicurezza**: Esposizione minima, isolamento, uso di utenti non-root, nessuna vulnerabilità nota.
- **On‑demand**: Capacità di monitorare lo stato dei container (via Docker socket) e di avviarli.
- **UI di attesa**: Una pagina web responsive, essenziale, con animazioni CSS molto leggere, che rassicuri l'utente durante l'avvio.
- **Semplicità di configurazione**: Tramite variabili d'ambiente o un file YAML, con supporto Docker Compose.

## 🏗️ Architettura proposta

Il sistema sarà composto da due componenti principali, idealmente in un unico container (per semplicità) oppure due container separati orchestrati con Docker Compose:

1. **Reverse proxy / traffic interceptor**  
   - Ascolta sulla porta 80/443.  
   - Controlla se l'host o il path richiesto corrisponde a un container gestito.  
   - Se il container è attivo → proxy pass diretto.  
   - Se spento → risponde con la pagina di attesa e contemporaneamente avvia il container (tramite API al manager).  

2. **Container manager**  
   - Comunica con il Docker socket (montato) per:  
     - Conoscere lo stato dei container (running/stopped).  
     - Avviare un container specifico.  
   - Espone una semplice API interna (es. `/start/<container>`).  

3. **UI statica**  
   - Pagina HTML+CSS minimale (no JS pesante, eventualmente un po' di JS per mostrare una barra di progresso fittizia).  
   - Servita dal proxy quando il container è spento.  

### Possibili stack tecnologici

- **Manager + Proxy**:  
  - **Go** con librerie standard (net/http, os/exec per docker, o client Docker SDK) → binario statico ~5 MB.  
  - **Rust** con atto-web o hyper → ancora più leggero ma curva di apprendimento.  
  - **Node.js** (sconsigliato per la leggerezza).  
  - **Python + Flask** (leggero ma > 50 MB con dipendenze).  
- **Proxy separato**:  
  - **Nginx** + Lua (con docker-cli) – più pesante, ma configurabile.  
  - **Traefik** con plugin on-demand – già visto, ma può essere overkill.  

La direzione attuale è sviluppare un unico eseguibile in **Go** che gestisca sia il proxy che il manager, minimizzando le risorse.

## 🎨 Design dell'UI di attesa

È stato preparato un prompt dettagliato per **Google Stitch** (gemini) per generare una UI responsive, minimal, con animazioni CSS delicate. Il prompt si trova in un file separato (`stitch-prompt.md`). L'idea è di avere:

- Header con logo e stato.
- Animazione di un "container che si risveglia" (pulsazione, linee ondulate).
- Messaggio "Stiamo avviando [nome container]".
- Barra di progresso sottile.
- Footer con info tecniche.

La UI sarà completamente statica (HTML/CSS) e potrà essere servita direttamente dal gateway.

## 📂 Stato attuale

- Idea concettuale definita.
- Prompt per la UI completato, in attesa di generare il design finale.
- Repository GitHub non ancora inizializzato (da fare).
- Decisioni tecniche ancora aperte (Go vs Rust, scelta del proxy).

## 🔜 Prossimi passi

1. **Decidere lo stack definitivo** (Go fortemente favorito).
2. **Generare il codice HTML/CSS** della UI tramite Stitch (o manualmente).
3. **Sviluppare il core in Go**:
   - Lettura configurazione (quali container gestire, timeouts, ecc.).
   - Monitoraggio stato container via Docker socket.
   - Endpoint che restituisce la UI con il nome container dinamicamente.
   - Logica di avvio container e attesa.
   - Reindirizzamento automatico dopo l'avvio.
4. **Integrare con un front-end server** (se usiamo un solo eseguibile, gestiremo anche le richieste HTTP).
5. **Test** con diversi container (nginx, whoami, ecc.).
6. **Packaging** in immagine Docker (multistage build, immagine finale su Alpine o scratch).
7. **Documentazione** e pubblicazione.

## 🤖 Come puoi aiutare (per gemini-cli)

- **Scrivere codice**: Implementare il manager in Go, gestire le chiamate Docker, creare il server HTTP.
- **Suggerire ottimizzazioni**: Per ridurre ulteriormente memoria e CPU.
- **Configurazioni**: Docker Compose di esempio, Dockerfile ottimizzato.
- **Sicurezza**: Best practice su come montare il socket Docker in modo sicuro.
- **Testing**: Script per simulare container spenti/avviati.
- **Documentazione**: README, istruzioni.

Grazie per il supporto! Ogni contributo è benvenuto.