// Global state
let currentUser = null;
let currentTab = 'tokens';

// Initialize on page load
document.addEventListener('DOMContentLoaded', function() {
    loadUserInfo();
    loadTokens();
    loadConfigurationSnippet();
});

// ==================== USER INFO ====================

async function loadUserInfo() {
    try {
        const response = await fetch('/api/auth/user', {
            credentials: 'include'
        });

        if (!response.ok) {
            window.location.href = '/admin/login';
            return;
        }

        currentUser = await response.json();

        // Update UI with user info
        document.getElementById('userEmail').textContent = currentUser.email;

        // Show admin badge if user is admin
        const adminBadge = document.getElementById('adminBadge');
        if (currentUser.is_admin) {
            adminBadge.innerHTML = '<span class="px-3 py-1 bg-gradient-to-r from-primary-600 to-primary-700 text-white text-xs font-semibold rounded-full shadow-sm">Admin</span>';
            // Show users tab for admins
            document.getElementById('usersTab').classList.remove('hidden');
        }
    } catch (error) {
        console.error('Failed to load user info:', error);
        window.location.href = '/admin/login';
    }
}

function logout() {
    fetch('/auth/logout', {
        method: 'POST',
        credentials: 'include'
    }).then(() => {
        window.location.href = '/admin/login';
    });
}

// ==================== TAB SWITCHING ====================

function switchTab(tab) {
    currentTab = tab;

    // Update tab UI - use Tailwind classes
    document.querySelectorAll('.nav-tab').forEach(t => {
        t.classList.remove('active', 'text-primary-600', 'border-primary-600');
        t.classList.add('text-apex-muted', 'border-transparent');
    });
    document.querySelectorAll('.tab-content').forEach(c => {
        c.classList.add('hidden');
        c.classList.remove('active');
    });

    // Activate clicked tab
    event.target.closest('.nav-tab').classList.add('active', 'text-primary-600', 'border-primary-600');
    event.target.closest('.nav-tab').classList.remove('text-apex-muted', 'border-transparent');
    document.getElementById(tab + 'Content').classList.remove('hidden');
    document.getElementById(tab + 'Content').classList.add('active');

    // Load tab content
    switch(tab) {
        case 'tokens':
            loadTokens();
            loadConfigurationSnippet();
            break;
        case 'analytics':
            loadAnalytics();
            break;
        case 'users':
            if (currentUser && currentUser.is_admin) {
                loadUsers();
            }
            break;
    }
}

// ==================== TOKENS MANAGEMENT ====================

async function loadTokens() {
    try {
        const response = await fetch('/api/auth/tokens', {
            credentials: 'include'
        });

        if (!response.ok) {
            throw new Error('Failed to load tokens');
        }

        const data = await response.json();
        displayTokens(data.tokens || []);
    } catch (error) {
        console.error('Error loading tokens:', error);
        document.getElementById('tokensContainer').innerHTML =
            '<div class="text-center py-12 text-apex-muted"><p>Failed to load tokens</p></div>';
    }
}

function displayTokens(tokens) {
    const container = document.getElementById('tokensContainer');

    if (tokens.length === 0) {
        container.innerHTML = `
            <div class="text-center py-16">
                <svg class="w-16 h-16 mx-auto mb-4 text-apex-muted opacity-30" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                </svg>
                <p class="text-apex-muted font-medium mb-2">No API tokens yet</p>
                <p class="text-sm text-apex-muted">Create your first token to get started</p>
            </div>
        `;
        return;
    }

    container.innerHTML = '<div class="space-y-4">' + tokens.map(token => {
        let statusBadge = '';
        let actionButton = '';

        if (token.revoked) {
            statusBadge = '<span class="px-3 py-1 bg-red-100 text-red-700 text-xs font-semibold rounded-full">Revoked</span>';
        } else if (token.expires_at && new Date(token.expires_at) < new Date()) {
            statusBadge = '<span class="px-3 py-1 bg-amber-100 text-amber-700 text-xs font-semibold rounded-full">Expired</span>';
        } else {
            statusBadge = '<span class="px-3 py-1 bg-green-100 text-green-700 text-xs font-semibold rounded-full">Active</span>';
            actionButton = `<button class="px-4 py-2 bg-red-50 hover:bg-red-100 text-red-600 font-medium rounded-lg transition-all duration-200 text-sm" onclick="revokeToken(${token.id})">Revoke</button>`;
        }

        return `
        <div class="border border-apex-border rounded-xl p-5 hover:shadow-apex-lg transition-all duration-200 bg-gradient-to-r from-white to-slate-50">
            <div class="flex justify-between items-start mb-3">
                <div>
                    <h3 class="text-lg font-semibold text-apex-text mb-2">${escapeHtml(token.name || 'Unnamed Token')}</h3>
                    <code class="px-3 py-1 bg-slate-100 text-slate-700 text-sm font-mono rounded-md">${escapeHtml(token.prefix)}...</code>
                </div>
                <div class="flex items-center space-x-3">
                    ${statusBadge}
                    ${actionButton}
                </div>
            </div>
            <div class="flex flex-wrap gap-4 text-sm text-apex-muted mt-4 pt-4 border-t border-apex-border">
                <span class="flex items-center">
                    <svg class="w-4 h-4 mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
                    </svg>
                    Created: ${formatDate(token.created_at)}
                </span>
                ${token.last_used_at ? `
                    <span class="flex items-center">
                        <svg class="w-4 h-4 mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                        Last used: ${formatDate(token.last_used_at)}
                    </span>
                ` : '<span class="text-amber-600">Never used</span>'}
                ${token.expires_at ? `
                    <span class="flex items-center">
                        <svg class="w-4 h-4 mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                        Expires: ${formatDate(token.expires_at)}
                    </span>
                ` : '<span class="text-green-600 font-medium">No expiration</span>'}
            </div>
        </div>
        `;
    }).join('') + '</div>';
}

function showCreateTokenModal() {
    document.getElementById('createTokenModal').classList.remove('hidden');
    document.getElementById('createTokenModal').classList.add('flex');
    document.getElementById('tokenCreationForm').classList.remove('hidden');
    document.getElementById('tokenCreatedDisplay').classList.add('hidden');
    document.getElementById('modalAlert').innerHTML = '';
    document.getElementById('tokenName').value = '';
    document.getElementById('tokenExpiry').value = '90';
}

function closeCreateTokenModal() {
    document.getElementById('createTokenModal').classList.add('hidden');
    document.getElementById('createTokenModal').classList.remove('flex');
}

async function createToken() {
    const name = document.getElementById('tokenName').value.trim();
    const expiresInDays = parseInt(document.getElementById('tokenExpiry').value) || 0;

    if (!name) {
        document.getElementById('modalAlert').innerHTML =
            '<div class="bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg mb-6">Please enter a token name</div>';
        return;
    }

    try {
        const response = await fetch('/api/auth/tokens', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ name, expires_in_days: expiresInDays })
        });

        const data = await response.json();

        if (!response.ok) {
            throw new Error(data.error?.message || 'Failed to create token');
        }

        // Show the token
        document.getElementById('tokenCreationForm').classList.add('hidden');
        document.getElementById('tokenCreatedDisplay').classList.remove('hidden');
        document.getElementById('newTokenValue').textContent = data.token;

        // Reload tokens list and configuration
        loadTokens();
        loadConfigurationSnippet();
    } catch (error) {
        document.getElementById('modalAlert').innerHTML =
            `<div class="bg-red-50 border border-red-200 text-red-800 px-4 py-3 rounded-lg mb-6">${escapeHtml(error.message)}</div>`;
    }
}

function copyToken() {
    const token = document.getElementById('newTokenValue').textContent;
    navigator.clipboard.writeText(token).then(() => {
        // Show toast notification
        const toast = document.createElement('div');
        toast.className = 'fixed top-4 right-4 bg-green-600 text-white px-6 py-3 rounded-lg shadow-lg z-50 animate-fade-in';
        toast.textContent = 'Token copied to clipboard!';
        document.body.appendChild(toast);
        setTimeout(() => toast.remove(), 3000);
    });
}

async function revokeToken(tokenId) {
    if (!confirm('Are you sure you want to revoke this token? This action cannot be undone.')) {
        return;
    }

    try {
        const response = await fetch(`/api/auth/tokens/${tokenId}`, {
            method: 'DELETE',
            credentials: 'include'
        });

        if (!response.ok) {
            throw new Error('Failed to revoke token');
        }

        loadTokens();

        // Show success toast
        const toast = document.createElement('div');
        toast.className = 'fixed top-4 right-4 bg-green-600 text-white px-6 py-3 rounded-lg shadow-lg z-50 animate-fade-in';
        toast.textContent = 'Token revoked successfully!';
        document.body.appendChild(toast);
        setTimeout(() => toast.remove(), 3000);
    } catch (error) {
        alert('Error: ' + error.message);
    }
}

// ==================== ANALYTICS ====================

async function loadAnalytics() {
    try {
        const response = await fetch('/api/auth/analytics?limit=50', {
            credentials: 'include'
        });

        if (!response.ok) {
            throw new Error('Failed to load analytics');
        }

        const data = await response.json();
        displayAnalytics(data);
    } catch (error) {
        console.error('Error loading analytics:', error);
        document.getElementById('recentLogs').innerHTML =
            '<div class="text-center py-12 text-apex-muted"><p>Failed to load analytics</p></div>';
    }
}

function displayAnalytics(data) {
    // Update stats
    document.getElementById('totalRequests').textContent = formatNumber(data.total_requests || 0);
    document.getElementById('totalTokens').textContent = formatNumber(data.total_tokens || 0);

    // Display recent logs
    const logsContainer = document.getElementById('recentLogs');
    const logs = data.recent_logs || [];

    if (logs.length === 0) {
        logsContainer.innerHTML = '<div class="text-center py-12 text-apex-muted"><p>No requests yet</p></div>';
    } else {
        logsContainer.innerHTML = `
            <table class="min-w-full divide-y divide-apex-border">
                <thead class="bg-slate-50">
                    <tr>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Time</th>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Model</th>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Provider</th>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Tokens</th>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Duration</th>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Status</th>
                    </tr>
                </thead>
                <tbody class="bg-white divide-y divide-apex-border">
                    ${logs.slice(0, 20).map(log => `
                        <tr class="hover:bg-slate-50 transition-colors duration-150">
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-apex-muted">${formatDateTime(log.timestamp)}</td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-apex-text">${escapeHtml(log.model)}</td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-apex-muted">${escapeHtml(log.provider)}</td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm font-mono text-apex-text">${formatNumber(log.total_tokens)}</td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-apex-muted">${log.duration}ms</td>
                            <td class="px-6 py-4 whitespace-nowrap">
                                <span class="px-3 py-1 inline-flex text-xs leading-5 font-semibold rounded-full ${log.status === 'success' ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'}">
                                    ${log.status}
                                </span>
                            </td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
    }

    // Display monthly summaries
    const summariesContainer = document.getElementById('monthlySummary');
    const summaries = data.summaries || [];

    if (summaries.length === 0) {
        summariesContainer.innerHTML = '<div class="text-center py-12 text-apex-muted"><p>No monthly data yet</p></div>';
    } else {
        summariesContainer.innerHTML = `
            <table class="min-w-full divide-y divide-apex-border">
                <thead class="bg-slate-50">
                    <tr>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Month</th>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Requests</th>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Success Rate</th>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Total Tokens</th>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Input Tokens</th>
                        <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Output Tokens</th>
                    </tr>
                </thead>
                <tbody class="bg-white divide-y divide-apex-border">
                    ${summaries.map(s => {
                        const successRate = s.total_requests > 0 ?
                            ((s.success_requests / s.total_requests) * 100).toFixed(1) : '0';
                        return `
                        <tr class="hover:bg-slate-50 transition-colors duration-150">
                            <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-apex-text">${s.year}-${String(s.month).padStart(2, '0')}</td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-apex-text">${formatNumber(s.total_requests)}</td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-apex-text">
                                <div class="flex items-center">
                                    <span class="font-medium">${successRate}%</span>
                                    <div class="ml-2 w-16 bg-gray-200 rounded-full h-2">
                                        <div class="bg-green-500 h-2 rounded-full" style="width: ${successRate}%"></div>
                                    </div>
                                </div>
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm font-mono text-apex-text">${formatNumber(s.total_tokens)}</td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm font-mono text-apex-muted">${formatNumber(s.total_input_tokens)}</td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm font-mono text-apex-muted">${formatNumber(s.total_output_tokens)}</td>
                        </tr>
                    `}).join('')}
                </tbody>
            </table>
        `;
    }
}

// ==================== CONFIGURATION ====================

async function loadConfigurationSnippet() {
    try {
        const response = await fetch('/api/auth/tokens', {
            credentials: 'include'
        });

        if (!response.ok) {
            throw new Error('Failed to load tokens');
        }

        const data = await response.json();
        const hasTokens = data.tokens && data.tokens.length > 0;

        displayConfigurationSnippet(hasTokens);
    } catch (error) {
        console.error('Error loading configuration:', error);
    }
}

function displayConfigurationSnippet(hasTokens) {
    const container = document.getElementById('configSection');

    if (!hasTokens) {
        container.innerHTML = `
            <div class="bg-blue-50 border border-blue-200 rounded-xl p-6">
                <div class="flex">
                    <svg class="w-6 h-6 text-blue-600 mr-3 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                    <div>
                        <h4 class="text-lg font-semibold text-blue-900 mb-2">Setup Configuration</h4>
                        <p class="text-blue-800">Create an API token below to get your configuration snippet for Claude Code.</p>
                    </div>
                </div>
            </div>
        `;
        return;
    }

    // Get dynamic base URL from current host
    const baseURL = window.location.origin;

    const configSnippet = `export ANTHROPIC_BASE_URL="${baseURL}"
export ANTHROPIC_API_KEY="your-token-here"`;

    container.innerHTML = `
        <div class="bg-white rounded-xl shadow-apex border border-apex-border overflow-hidden">
            <div class="px-6 py-5 border-b border-apex-border bg-gradient-to-r from-green-50 to-emerald-50">
                <div class="flex items-start justify-between">
                    <div class="flex">
                        <svg class="w-6 h-6 text-green-600 mr-3 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                        <div>
                            <h3 class="text-lg font-bold text-green-900 mb-1">Configuration</h3>
                            <p class="text-sm text-green-800">Add these environment variables to your shell profile (~/.bashrc, ~/.zshrc)</p>
                        </div>
                    </div>
                </div>
            </div>
            <div class="p-6">
                <div class="relative">
                    <div class="bg-slate-900 text-slate-100 rounded-xl p-6 overflow-x-auto shadow-lg">
                        <pre class="text-sm font-mono leading-relaxed">${escapeHtml(configSnippet)}</pre>
                    </div>
                    <button class="absolute top-4 right-4 px-4 py-2 bg-slate-700 hover:bg-slate-600 text-white text-sm font-medium rounded-lg transition-all duration-200 shadow-md flex items-center space-x-2" onclick="copyConfigSnippet()">
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                        </svg>
                        <span>Copy</span>
                    </button>
                </div>
                <div class="mt-4 p-4 bg-amber-50 border border-amber-200 rounded-lg">
                    <p class="text-sm text-amber-800">
                        <strong>Note:</strong> Replace <code class="px-2 py-0.5 bg-amber-100 rounded font-mono text-xs">your-token-here</code> with one of your API tokens from below.
                    </p>
                </div>
            </div>
        </div>
    `;
}

function copyConfigSnippet() {
    const baseURL = window.location.origin;
    const configSnippet = `export ANTHROPIC_BASE_URL="${baseURL}"
export ANTHROPIC_API_KEY="your-token-here"`;

    navigator.clipboard.writeText(configSnippet).then(() => {
        const toast = document.createElement('div');
        toast.className = 'fixed top-4 right-4 bg-green-600 text-white px-6 py-3 rounded-lg shadow-lg z-50 animate-fade-in';
        toast.textContent = 'Configuration copied to clipboard!';
        document.body.appendChild(toast);
        setTimeout(() => toast.remove(), 3000);
    });
}

// ==================== USERS MANAGEMENT (ADMIN ONLY) ====================

async function loadUsers() {
    if (!currentUser || !currentUser.is_admin) {
        return;
    }

    try {
        const response = await fetch('/api/admin/analytics', {
            credentials: 'include'
        });

        if (!response.ok) {
            throw new Error('Failed to load users');
        }

        const data = await response.json();
        displayUsers(data);
    } catch (error) {
        console.error('Error loading users:', error);
        document.getElementById('usersContainer').innerHTML =
            '<div class="text-center py-12 text-apex-muted"><p>Failed to load users</p></div>';
    }
}

function displayUsers(data) {
    // Update stats
    const statsGrid = document.getElementById('usersStatsGrid');
    statsGrid.innerHTML = `
        <div class="bg-white rounded-xl shadow-apex border border-apex-border p-6 hover:shadow-apex-lg transition-shadow duration-200">
            <div class="flex items-center justify-between">
                <div>
                    <p class="text-sm font-medium text-apex-muted mb-1">Total Users</p>
                    <p class="text-3xl font-bold text-apex-text">${formatNumber(data.total_users || 0)}</p>
                </div>
                <div class="w-12 h-12 bg-gradient-to-br from-indigo-100 to-indigo-200 rounded-xl flex items-center justify-center">
                    <svg class="w-6 h-6 text-indigo-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
                    </svg>
                </div>
            </div>
        </div>
        <div class="bg-white rounded-xl shadow-apex border border-apex-border p-6 hover:shadow-apex-lg transition-shadow duration-200">
            <div class="flex items-center justify-between">
                <div>
                    <p class="text-sm font-medium text-apex-muted mb-1">Total Requests</p>
                    <p class="text-3xl font-bold text-apex-text">${formatNumber(data.total_requests || 0)}</p>
                </div>
                <div class="w-12 h-12 bg-gradient-to-br from-blue-100 to-blue-200 rounded-xl flex items-center justify-center">
                    <svg class="w-6 h-6 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z" />
                    </svg>
                </div>
            </div>
        </div>
        <div class="bg-white rounded-xl shadow-apex border border-apex-border p-6 hover:shadow-apex-lg transition-shadow duration-200">
            <div class="flex items-center justify-between">
                <div>
                    <p class="text-sm font-medium text-apex-muted mb-1">Total Tokens</p>
                    <p class="text-3xl font-bold text-apex-text">${formatNumber(data.total_tokens || 0)}</p>
                </div>
                <div class="w-12 h-12 bg-gradient-to-br from-purple-100 to-purple-200 rounded-xl flex items-center justify-center">
                    <svg class="w-6 h-6 text-purple-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                    </svg>
                </div>
            </div>
        </div>
    `;

    // Display users table
    const container = document.getElementById('usersContainer');
    const users = data.users || [];

    if (users.length === 0) {
        container.innerHTML = '<div class="text-center py-12 text-apex-muted"><p>No users yet</p></div>';
        return;
    }

    container.innerHTML = `
        <table class="min-w-full divide-y divide-apex-border">
            <thead class="bg-slate-50">
                <tr>
                    <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Email</th>
                    <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Name</th>
                    <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Role</th>
                    <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Joined</th>
                    <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Last Login</th>
                    <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Requests</th>
                    <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Tokens</th>
                    <th class="px-6 py-4 text-left text-xs font-semibold text-apex-text uppercase tracking-wider">Actions</th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-apex-border">
                ${users.map(user => `
                    <tr class="hover:bg-slate-50 transition-colors duration-150">
                        <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-apex-text">${escapeHtml(user.email)}</td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-apex-muted">${escapeHtml(user.name)}</td>
                        <td class="px-6 py-4 whitespace-nowrap">
                            ${user.is_admin ?
                                '<span class="px-3 py-1 bg-gradient-to-r from-primary-600 to-primary-700 text-white text-xs font-semibold rounded-full">Admin</span>' :
                                '<span class="px-3 py-1 bg-blue-100 text-blue-700 text-xs font-semibold rounded-full">User</span>'}
                        </td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-apex-muted">${formatDate(user.created_at)}</td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm text-apex-muted">${user.last_login_at ? formatDate(user.last_login_at) : 'Never'}</td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm font-mono text-apex-text">${formatNumber(user.total_requests)}</td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm font-mono text-apex-text">${formatNumber(user.total_tokens)}</td>
                        <td class="px-6 py-4 whitespace-nowrap text-sm">
                            ${user.user_id !== currentUser.id ? (
                                user.is_admin ?
                                    `<button class="px-4 py-2 bg-slate-100 hover:bg-slate-200 text-slate-700 font-medium rounded-lg transition-all duration-200" onclick="demoteUser(${user.user_id})">Demote</button>` :
                                    `<button class="px-4 py-2 bg-green-50 hover:bg-green-100 text-green-600 font-medium rounded-lg transition-all duration-200" onclick="promoteUser(${user.user_id})">Promote</button>`
                            ) : '<span class="text-apex-muted text-xs font-medium">You</span>'}
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;
}

async function promoteUser(userId) {
    if (!confirm('Promote this user to admin? They will have full access to all admin features.')) {
        return;
    }

    try {
        const response = await fetch(`/api/admin/users/${userId}/promote`, {
            method: 'POST',
            credentials: 'include'
        });

        if (!response.ok) {
            throw new Error('Failed to promote user');
        }

        const toast = document.createElement('div');
        toast.className = 'fixed top-4 right-4 bg-green-600 text-white px-6 py-3 rounded-lg shadow-lg z-50 animate-fade-in';
        toast.textContent = 'User promoted to admin successfully!';
        document.body.appendChild(toast);
        setTimeout(() => toast.remove(), 3000);

        loadUsers();
    } catch (error) {
        alert('Error: ' + error.message);
    }
}

async function demoteUser(userId) {
    if (!confirm('Remove admin privileges from this user?')) {
        return;
    }

    try {
        const response = await fetch(`/api/admin/users/${userId}/demote`, {
            method: 'POST',
            credentials: 'include'
        });

        if (!response.ok) {
            throw new Error('Failed to demote user');
        }

        const toast = document.createElement('div');
        toast.className = 'fixed top-4 right-4 bg-green-600 text-white px-6 py-3 rounded-lg shadow-lg z-50 animate-fade-in';
        toast.textContent = 'Admin privileges removed successfully!';
        document.body.appendChild(toast);
        setTimeout(() => toast.remove(), 3000);

        loadUsers();
    } catch (error) {
        alert('Error: ' + error.message);
    }
}

// ==================== UTILITY FUNCTIONS ====================

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function formatDate(dateString) {
    const date = new Date(dateString);
    return date.toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric'
    });
}

function formatDateTime(dateString) {
    const date = new Date(dateString);
    return date.toLocaleString('en-US', {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
    });
}

function formatNumber(num) {
    return num.toLocaleString('en-US');
}

// Close modal when clicking outside
document.addEventListener('click', function(event) {
    const modal = document.getElementById('createTokenModal');
    if (event.target === modal) {
        closeCreateTokenModal();
    }
});
