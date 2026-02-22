# Docker Awakening Gateway - Piano di Sviluppo

Questo documento delinea le fasi di sviluppo per il progetto "Docker Awakening Gateway", un proxy intelligente e leggero per la gestione on-demand dei container Docker.

## 🎯 Obiettivi Chiave
1.  **Leggerezza Estrema:** Immagine Docker finale < 20MB (Go binary statico + risorse embeddate).
2.  **Nessuna Dipendenza Runtime:** Il container finale non deve dipendere da CDN esterne (Tailwind deve essere compilato/inlinato) né librerie di sistema pesanti.
3.  **Resilienza:** Gestione sicura delle richieste concorrenti (evitare avvii multipli dello stesso container).
4.  **Esperienza Utente:** Fedeltà ai mockup "Precision Grid" forniti.

## 🛠 Stack Tecnologico Proposto
*   **Backend:** Go (Golang) - Ideale per performance, concorrenza e binari statici.
*   **Frontend:** HTML/CSS statico (estratto dai mockup).
*   **Docker Integration:** Docker Client SDK per Go.
*   **Deployment:** Docker Multi-stage build (Build in immagine Go -> Deploy su `scratch` o `alpine`).

---

## 📅 Fasi di Sviluppo

### FASE 1: Architettura e Core Backend
*Obiettivo: Avere un server HTTP che parla col demone Docker.*

1.  **Setup Progetto Go:** Inizializzazione modulo e dipendenze (Docker SDK).
2.  **Docker Client:** Implementazione della connessione al socket Docker (`/var/run/docker.sock`).
3.  **Risoluzione Target:** Logica per mappare l'hostname o il path della richiesta al nome del container target (es. tramite label Docker o file di config semplice).
4.  **Status Check:** Funzione per interrogare lo stato del container (Running vs Exited).

### FASE 2: Logica di "Risveglio" (The Awakening)
*Obiettivo: Gestire il flusso di avvio senza bloccare il server.*

1.  **Interceptor Middleware:**
    *   Se Container è UP -> Proxy inverso trasparente verso l'IP/Porta del container.
    *   Se Container è DOWN -> Avvia procedura di start e serve la pagina di caricamento.
2.  **Concurrency Control:** Implementazione di un sistema di lock/mutex per evitare che 100 richieste simultanee provino ad avviare lo stesso container 100 volte.
3.  **Start Routine:** Chiamata API per avviare il container e attesa attiva (polling) finché non è "Healthy" o "Running".

### FASE 3: Integrazione Frontend (Stitch Mockups)
*Obiettivo: Trasformare i mockup in template performanti.*

1.  **CSS Optimization:** I mockup usano Tailwind via CDN. Per la produzione:
    *   Estrarre il CSS critico o usare la CLI di Tailwind per generare un singolo file `.css` minificato.
    *   Eliminare lo script JS di Tailwind dal file HTML finale.
2.  **Template Embedding:** Usare `go:embed` per includere i file HTML/CSS direttamente nel binario Go (zero file esterni al runtime).
3.  **Dynamic Rendering:** Adattare l'HTML per iniettare dinamicamente:
    *   Nome del Container.
    *   Stato (Awakening/Error).
    *   Request ID.
4.  **Client-Side Logic:** Implementare un leggerissimo script JS (vanilla, inline) nella pagina di attesa che fa polling verso un endpoint di stato del gateway per sapere quando fare il reload/redirect (evitando il refresh manuale).

### FASE 4: Robustezza e Proxy
*Obiettivo: Il gateway deve funzionare come un vero proxy.*

1.  **Reverse Proxy:** Implementazione di `httputil.ReverseProxy` in Go per inoltrare il traffico una volta che il container è attivo.
2.  **Error Handling:** Gestione dei timeout (se il container non parte in X secondi) e visualizzazione della pagina "Failure State" (dai mockup).
3.  **Configurazione:** Supporto per variabili d'ambiente (es. porta di ascolto, timeout, whitelist container).

### FASE 5: Dockerizzazione Finale
*Obiettivo: Build di produzione.*

1.  **Dockerfile Multi-stage:**
    *   *Builder:* Compila il binario Go (flag `CGO_ENABLED=0` per link statico).
    *   *Runner:* Immagine `scratch` (vuota) o `gcr.io/distroless/static`.
    *   Copia del binario e dei certificati CA (per chiamate HTTPS se necessarie).
2.  **Security:** Il container girerà come utente non-root (ove possibile, tenendo conto dell'accesso al socket Docker).
