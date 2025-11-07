// API base URL
const API_BASE = '/api/auth';

// Initialize the dashboard
document.addEventListener('DOMContentLoaded', function() {
    loadUser();
    loadTokens();
});

// Load user information
async function loadUser() {
    try {
        const response = await fetch(`${API_BASE}/user`);
        if (response.ok) {
            const data = await response.json();
            document.getElementById('userEmail').textContent = data.email;
        } else {
            // Not authenticated, redirect to login
            window.location.href = '/admin/login';
        }
    } catch (error) {
        console.error('Failed to load user:', error);
        showAlert('Failed to load user information', 'error');
    }
}

// Load tokens
async function loadTokens() {
    const container = document.getElementById('tokensContainer');

    try {
        const response = await fetch(`${API_BASE}/tokens`);
        if (!response.ok) {
            throw new Error('Failed to load tokens');
        }

        const data = await response.json();
        const tokens = data.tokens || [];

        if (tokens.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                        <rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect>
                        <path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
                    </svg>
                    <h3 style="margin-bottom: 8px;">No API tokens yet</h3>
                    <p>Create your first token to start using the proxy</p>
                </div>
            `;
        } else {
            container.innerHTML = `
                <div class="tokens-grid">
                    ${tokens.map(token => renderToken(token)).join('')}
                </div>
            `;
        }
    } catch (error) {
        console.error('Failed to load tokens:', error);
        container.innerHTML = `
            <div class="alert alert-error">
                Failed to load tokens. Please try again.
            </div>
        `;
    }
}

// Render a single token
function renderToken(token) {
    const isValid = token.is_valid;
    const isExpired = token.expires_at && new Date(token.expires_at) < new Date();
    const status = token.revoked ? 'revoked' : (isExpired ? 'expired' : 'active');

    const statusBadge = {
        'active': '<span class="badge badge-success">Active</span>',
        'revoked': '<span class="badge badge-danger">Revoked</span>',
        'expired': '<span class="badge badge-warning">Expired</span>'
    }[status];

    const lastUsed = token.last_used_at
        ? `Last used: ${formatDate(token.last_used_at)}`
        : 'Never used';

    const expires = token.expires_at
        ? `Expires: ${formatDate(token.expires_at)}`
        : 'Never expires';

    return `
        <div class="token-item">
            <div class="token-info">
                <h3>${escapeHtml(token.name)} ${statusBadge}</h3>
                <div class="token-meta">
                    <span class="token-prefix">${escapeHtml(token.prefix)}</span>
                    <span>Created: ${formatDate(token.created_at)}</span>
                    <span>${lastUsed}</span>
                    <span>${expires}</span>
                </div>
            </div>
            <div class="token-actions">
                ${isValid ? `
                    <button class="btn btn-danger" onclick="revokeToken(${token.id}, '${escapeHtml(token.name)}')">
                        Revoke
                    </button>
                ` : ''}
            </div>
        </div>
    `;
}

// Show create token modal
function showCreateTokenModal() {
    document.getElementById('createTokenModal').classList.add('active');
    document.getElementById('tokenCreationForm').style.display = 'block';
    document.getElementById('tokenCreatedDisplay').style.display = 'none';
    document.getElementById('modalAlert').innerHTML = '';
    document.getElementById('tokenName').value = '';
    document.getElementById('tokenExpiry').value = '90';
}

// Close create token modal
function closeCreateTokenModal() {
    document.getElementById('createTokenModal').classList.remove('active');
}

// Create token
async function createToken() {
    const name = document.getElementById('tokenName').value.trim();
    const expiresInDays = parseInt(document.getElementById('tokenExpiry').value) || 0;

    if (!name) {
        showModalAlert('Please enter a token name', 'error');
        return;
    }

    if (expiresInDays < 0) {
        showModalAlert('Expiration days cannot be negative', 'error');
        return;
    }

    try {
        const response = await fetch(`${API_BASE}/tokens`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                name: name,
                expiresInDays: expiresInDays
            })
        });

        const data = await response.json();

        if (!response.ok) {
            throw new Error(data.error?.message || 'Failed to create token');
        }

        // Show the token
        document.getElementById('tokenCreationForm').style.display = 'none';
        document.getElementById('tokenCreatedDisplay').style.display = 'block';
        document.getElementById('newTokenValue').textContent = data.token;

        // Reload tokens list
        loadTokens();
    } catch (error) {
        console.error('Failed to create token:', error);
        showModalAlert(error.message, 'error');
    }
}

// Copy token to clipboard
function copyToken() {
    const tokenValue = document.getElementById('newTokenValue').textContent;
    navigator.clipboard.writeText(tokenValue).then(() => {
        showModalAlert('Token copied to clipboard!', 'success');
    }).catch(err => {
        console.error('Failed to copy:', err);
        showModalAlert('Failed to copy token', 'error');
    });
}

// Revoke token
async function revokeToken(tokenId, tokenName) {
    if (!confirm(`Are you sure you want to revoke the token "${tokenName}"? This action cannot be undone.`)) {
        return;
    }

    try {
        const response = await fetch(`${API_BASE}/tokens/${tokenId}`, {
            method: 'DELETE'
        });

        if (!response.ok) {
            const data = await response.json();
            throw new Error(data.error?.message || 'Failed to revoke token');
        }

        // Reload tokens
        loadTokens();
    } catch (error) {
        console.error('Failed to revoke token:', error);
        alert('Failed to revoke token: ' + error.message);
    }
}

// Logout
async function logout() {
    try {
        await fetch('/auth/logout', { method: 'POST' });
    } catch (error) {
        console.error('Logout error:', error);
    } finally {
        window.location.href = '/admin/login';
    }
}

// Show modal alert
function showModalAlert(message, type) {
    const alertDiv = document.getElementById('modalAlert');
    alertDiv.innerHTML = `
        <div class="alert alert-${type === 'error' ? 'error' : 'success'}">
            ${escapeHtml(message)}
        </div>
    `;
}

// Format date
function formatDate(dateString) {
    const date = new Date(dateString);
    const now = new Date();
    const diff = now - date;
    const days = Math.floor(diff / (1000 * 60 * 60 * 24));

    if (days === 0) {
        const hours = Math.floor(diff / (1000 * 60 * 60));
        if (hours === 0) {
            const minutes = Math.floor(diff / (1000 * 60));
            return minutes === 0 ? 'Just now' : `${minutes}m ago`;
        }
        return `${hours}h ago`;
    } else if (days === 1) {
        return 'Yesterday';
    } else if (days < 7) {
        return `${days}d ago`;
    } else {
        return date.toLocaleDateString();
    }
}

// Escape HTML to prevent XSS
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Close modal when clicking outside
document.addEventListener('click', function(event) {
    const modal = document.getElementById('createTokenModal');
    if (event.target === modal) {
        closeCreateTokenModal();
    }
});
