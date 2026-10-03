// WS.js - WebSocket helper with exponential backoff
class WSHelper {
    constructor(url, options = {}) {
        this.url = url;
        this.options = {
            reconnectInterval: 1000,
            maxReconnectAttempts: 5,
            reconnectDecay: 1.5,
            timeoutInterval: 2000,
            ...options
        };
        
        this.reconnectAttempts = 0;
        this.reconnectTimeoutId = null;
        this.pingTimeoutId = null;
        this.isConnected = false;
        this.messageQueue = [];
        
        this.connect();
    }

    connect() {
        try {
            this.ws = new WebSocket(this.url);
            this.setupEventHandlers();
        } catch (error) {
            console.error('WebSocket connection failed:', error);
            this.handleReconnect();
        }
    }

    setupEventHandlers() {
        this.ws.onopen = (event) => {
            console.log('WebSocket connected');
            this.isConnected = true;
            this.reconnectAttempts = 0;
            this.startPing();
            this.flushMessageQueue();
            
            if (this.options.onOpen) {
                this.options.onOpen(event);
            }
        };

        this.ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                
                if (this.options.onMessage) {
                    this.options.onMessage(message);
                }
            } catch (error) {
                console.error('Failed to parse WebSocket message:', error);
            }
        };

        this.ws.onclose = (event) => {
            console.log('WebSocket disconnected:', event.code, event.reason);
            this.isConnected = false;
            this.stopPing();
            
            if (this.options.onClose) {
                this.options.onClose(event);
            }
            
            // Don't reconnect if it was a clean close
            if (event.code !== 1000) {
                this.handleReconnect();
            }
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            
            if (this.options.onError) {
                this.options.onError(error);
            }
        };
    }

    handleReconnect() {
        if (this.reconnectAttempts >= this.options.maxReconnectAttempts) {
            console.error('Max reconnection attempts reached');
            
            if (this.options.onMaxReconnectAttempts) {
                this.options.onMaxReconnectAttempts();
            }
            return;
        }

        this.reconnectAttempts++;
        const delay = this.options.reconnectInterval * Math.pow(this.options.reconnectDecay, this.reconnectAttempts - 1);
        
        console.log(`Attempting to reconnect in ${delay}ms (attempt ${this.reconnectAttempts}/${this.options.maxReconnectAttempts})`);
        
        this.reconnectTimeoutId = setTimeout(() => {
            this.connect();
        }, delay);
    }

    startPing() {
        this.pingTimeoutId = setInterval(() => {
            if (this.isConnected && this.ws.readyState === WebSocket.OPEN) {
                this.ws.ping();
            }
        }, this.options.timeoutInterval);
    }

    stopPing() {
        if (this.pingTimeoutId) {
            clearInterval(this.pingTimeoutId);
            this.pingTimeoutId = null;
        }
    }

    send(message) {
        if (this.isConnected && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify(message));
        } else {
            // Queue message for when connection is restored
            this.messageQueue.push(message);
        }
    }

    flushMessageQueue() {
        while (this.messageQueue.length > 0) {
            const message = this.messageQueue.shift();
            this.send(message);
        }
    }

    close() {
        this.stopPing();
        
        if (this.reconnectTimeoutId) {
            clearTimeout(this.reconnectTimeoutId);
        }
        
        if (this.ws) {
            this.ws.close(1000, 'Client disconnect');
        }
    }

    // Static method to create WebSocket with JWT authentication
    static createAuthenticated(url, token, options = {}) {
        const wsUrl = new URL(url);
        wsUrl.searchParams.set('token', token);
        
        return new WSHelper(wsUrl.toString(), options);
    }
}

// Make WSHelper available globally
window.WSHelper = WSHelper;
