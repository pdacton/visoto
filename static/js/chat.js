// Chat functionality for the right sidebar AI assistant

(function() {
  'use strict';

  let resourceData = null;
  let chatHistory = [];
  let isProcessing = false;

  // Initialize chat when DOM is ready
  document.addEventListener('DOMContentLoaded', function() {
    // Only initialize if we have resource data
    const dataElement = document.getElementById('resource-data');
    if (!dataElement) {
      // No resource page, show a message
      showNoResourceMessage();
      return;
    }

    try {
      // The data is double-JSON-encoded, so we need to parse twice
      let rawData = JSON.parse(dataElement.textContent);
      // If it's a string, parse it again
      if (typeof rawData === 'string') {
        rawData = JSON.parse(rawData);
      }
      resourceData = rawData;
    } catch (e) {
      console.error('Failed to parse resource data:', e);
      showNoResourceMessage();
      return;
    }

    initializeChat();
  });

  function initializeChat() {
    const chatForm = document.getElementById('chat-form');
    const chatInput = document.getElementById('chat-input');
    const chatMessages = document.getElementById('chat-messages');
    const clearButton = document.getElementById('chat-clear');

    if (!chatForm || !chatInput || !chatMessages) {
      return;
    }

    // Load chat history from localStorage
    loadChatHistory();

    // If we have history, render it
    if (chatHistory.length > 0) {
      renderHistory();
    } else {
      // Only trigger auto-summary if the sidebar is currently open
      if (localStorage.getItem('rightSidebarOpen') === 'true') {
        generateAutoSummary();
      } else {
        // Defer auto-summary until the sidebar is opened for the first time
        const toggle = document.getElementById('right-sidebar-toggle');
        if (toggle) {
          toggle.addEventListener('click', function onFirstOpen() {
            toggle.removeEventListener('click', onFirstOpen);
            // Small delay to let the sidebar finish opening
            setTimeout(function() {
              if (chatHistory.length === 0 && !isProcessing) {
                generateAutoSummary();
              }
            }, 300);
          });
        }
      }
    }

    // Handle form submission
    chatForm.addEventListener('submit', function(e) {
      e.preventDefault();
      const message = chatInput.value.trim();
      if (message && !isProcessing) {
        sendMessage(message);
        chatInput.value = '';
      }
    });

    // Handle clear button
    if (clearButton) {
      clearButton.addEventListener('click', function() {
        if (confirm('Clear all chat history for this resource?')) {
          clearChatHistory();
        }
      });
    }
  }

  function loadChatHistory() {
    const key = getChatStorageKey();
    const stored = localStorage.getItem(key);
    if (stored) {
      try {
        const data = JSON.parse(stored);
        chatHistory = data.messages || [];
      } catch (e) {
        console.error('Failed to load chat history:', e);
        chatHistory = [];
      }
    }
  }

  function saveChatHistory() {
    const key = getChatStorageKey();
    const data = {
      resourceIRI: resourceData.ResourceIRI,
      messages: chatHistory.slice(-50), // Keep last 50 messages
      timestamp: new Date().toISOString()
    };
    localStorage.setItem(key, JSON.stringify(data));
  }

  function getChatStorageKey() {
    // Create a safe key from the resource IRI
    const iri = resourceData.ResourceIRI;
    return `chat_history_${btoa(iri).replace(/[^a-zA-Z0-9]/g, '_')}`;
  }

  function clearChatHistory() {
    chatHistory = [];
    const key = getChatStorageKey();
    localStorage.removeItem(key);

    // Clear UI
    const chatMessages = document.getElementById('chat-messages');
    chatMessages.innerHTML = '';

    // Regenerate summary
    generateAutoSummary();
  }

  function renderHistory() {
    const chatMessages = document.getElementById('chat-messages');
    const initial = document.getElementById('chat-initial');
    if (initial) {
      initial.remove();
    }

    chatMessages.innerHTML = '';

    chatHistory.forEach(msg => {
      appendMessage(msg.role, msg.content, false);
    });

    scrollToBottom();
  }

  function generateAutoSummary() {
    const summaryMessage = 'Please provide a 2-sentence summary of this resource.';
    sendMessage(summaryMessage, true);
  }

  function sendMessage(message, isAutoSummary = false) {
    if (isProcessing) return;

    isProcessing = true;

    // Add user message to UI (unless it's auto-summary)
    if (!isAutoSummary) {
      addUserMessage(message);
      chatHistory.push({ role: 'user', content: message });
    }

    // Show loading indicator
    showLoadingIndicator();

    // Disable input
    const chatInput = document.getElementById('chat-input');
    const chatSend = document.getElementById('chat-send');
    if (chatInput) chatInput.disabled = true;
    if (chatSend) chatSend.disabled = true;

    // Get accept language from browser
    const acceptLanguage = navigator.language || 'en';

    // Resolve the active endpoint via the shared slug resolver + page data
    const selectedSlug = (typeof activeEndpointSlug === 'function') ? activeEndpointSlug() : '';
    const endpoints = resourceData.SparqlEndpoints || [];
    let activeEndpoint = { name: '', url: '' };
    if (selectedSlug) {
      const match = endpoints.find(ep => ep.Slug === selectedSlug);
      if (match) activeEndpoint = { name: match.Name, url: match.URL };
    }
    if (!activeEndpoint.name) {
      // Fall back to the default endpoint
      const def = endpoints.find(ep => ep.Default) || endpoints[0];
      if (def) activeEndpoint = { name: def.Name, url: def.URL };
    }

    // Prepare request
    const requestData = {
      resourceIRI: resourceData.ResourceIRI,
      message: message,
      history: chatHistory,
      data: resourceData.QueryResults || {},
      acceptLanguage: acceptLanguage,
      activeEndpoint: activeEndpoint
    };

    // Call API
    fetch('/api/chat', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(requestData)
    })
    .then(response => {
      // Check if it's a service unavailable error (API key not configured)
      if (response.status === 503) {
        hideLoadingIndicator();
        showConfigurationMessage();
        // Return a rejected promise to skip the next .then() block
        return Promise.reject(new Error('Service not configured'));
      }
      return response.json();
    })
    .then(data => {
      hideLoadingIndicator();

      if (data.error) {
        addErrorMessage(data.error);
      } else {
        addAssistantMessage(data.response);
        chatHistory.push({ role: 'assistant', content: data.response });
        saveChatHistory();
      }
    })
    .catch(error => {
      console.error('Chat API error:', error);
      hideLoadingIndicator();
      // Don't show error if it's the configuration error (already shown)
      if (error.message !== 'Service not configured') {
        addErrorMessage('Failed to connect to chat service. Please try again.');
      }
    })
    .finally(() => {
      isProcessing = false;
      if (chatInput) chatInput.disabled = false;
      if (chatSend) chatSend.disabled = false;
      if (chatInput) chatInput.focus();
    });
  }

  function addUserMessage(content) {
    appendMessage('user', content, true);
  }

  function addAssistantMessage(content) {
    appendMessage('assistant', content, true);
  }

  function addErrorMessage(content) {
    const chatMessages = document.getElementById('chat-messages');
    const errorDiv = document.createElement('div');
    errorDiv.className = 'alert alert-danger mb-3';
    errorDiv.innerHTML = `<strong>Error:</strong> ${escapeHtml(content)}`;
    chatMessages.appendChild(errorDiv);
    scrollToBottom();
  }

  function showNoResourceMessage() {
    const chatMessages = document.getElementById('chat-messages');
    if (!chatMessages) return;

    // Remove initial placeholder if present
    const initial = document.getElementById('chat-initial');
    if (initial) {
      initial.remove();
    }

    const infoDiv = document.createElement('div');
    infoDiv.className = 'alert alert-info mb-3';
    infoDiv.innerHTML = `
      <h4 class="alert-heading"><i data-lucide="info" class="me-2"></i>${vsT('js.chat.title', 'Resource Assistant')}</h4>
      <p>${vsT('js.chat.intro', 'The chat assistant helps you understand RDF resources.')}</p>
      <p class="mb-0">${vsT('js.chat.noResource', 'Navigate to a resource page to start chatting!')}</p>
    `;
    chatMessages.appendChild(infoDiv);

    // Re-initialize lucide icons
    if (typeof lucide !== 'undefined') {
      lucide.createIcons();
    }

    scrollToBottom();
  }

  function showConfigurationMessage() {
    const chatMessages = document.getElementById('chat-messages');

    // Remove initial placeholder if present
    const initial = document.getElementById('chat-initial');
    if (initial) {
      initial.remove();
    }

    const configDiv = document.createElement('div');
    configDiv.className = 'alert alert-warning mb-3';
    configDiv.innerHTML = `
      <h4 class="alert-heading"><i data-lucide="alert-circle" class="me-2"></i>Chat Not Configured</h4>
      <p>The AI chat assistant requires a Google Gemini API key to function.</p>
      <hr>
      <p class="mb-2"><strong>To enable the chat:</strong></p>
      <ol class="mb-3">
        <li>Visit <a href="https://aistudio.google.com/app/apikey" target="_blank" rel="noopener">Google AI Studio</a></li>
        <li>Generate a free Gemini API key</li>
        <li>Add the key to <code>visoto.config</code>:
          <pre class="mt-2 mb-0"><code>gemini_api_key = "YOUR_API_KEY_HERE"</code></pre>
        </li>
        <li>Restart the server</li>
      </ol>
      <p class="mb-0 text-muted small">The API key is stored securely on the server and never exposed to the browser.</p>
    `;
    chatMessages.appendChild(configDiv);

    // Re-initialize lucide icons for the new alert icon
    if (typeof lucide !== 'undefined') {
      lucide.createIcons();
    }

    scrollToBottom();
  }

  function appendMessage(role, content, shouldScroll) {
    const chatMessages = document.getElementById('chat-messages');

    // Remove initial placeholder if present
    const initial = document.getElementById('chat-initial');
    if (initial) {
      initial.remove();
    }

    const messageDiv = document.createElement('div');
    messageDiv.className = `chat-message ${role}`;

    const bubbleDiv = document.createElement('div');
    bubbleDiv.className = `chat-bubble ${role}`;

    // Parse markdown and convert links
    bubbleDiv.innerHTML = parseMarkdown(content);

    messageDiv.appendChild(bubbleDiv);
    chatMessages.appendChild(messageDiv);

    if (shouldScroll) {
      scrollToBottom();
    }
  }

  function showLoadingIndicator() {
    const chatMessages = document.getElementById('chat-messages');
    const loadingDiv = document.createElement('div');
    loadingDiv.id = 'chat-loading';
    loadingDiv.className = 'chat-message assistant';
    loadingDiv.innerHTML = `
      <div class="chat-bubble assistant">
        <div class="spinner-border spinner-border-sm me-2" role="status">
          <span class="visually-hidden">Loading...</span>
        </div>
        <span class="text-muted">Thinking...</span>
      </div>
    `;
    chatMessages.appendChild(loadingDiv);
    scrollToBottom();
  }

  function hideLoadingIndicator() {
    const loading = document.getElementById('chat-loading');
    if (loading) {
      loading.remove();
    }
    // Also remove the initial "Analyzing resource..." placeholder if still present
    const initial = document.getElementById('chat-initial');
    if (initial) {
      initial.remove();
    }
  }

  function scrollToBottom() {
    const chatMessages = document.getElementById('chat-messages');
    if (chatMessages) {
      chatMessages.scrollTop = chatMessages.scrollHeight;
    }
  }

  function parseMarkdown(text) {
    // Simple markdown parsing for bold, italic, and links
    let html = escapeHtml(text);

    // Convert markdown links [text](url) to HTML
    html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>');

    // Bold **text**
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');

    // Italic *text*
    html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');

    // Line breaks
    html = html.replace(/\n/g, '<br>');

    return html;
  }

  function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

})();
