// Check for encryption support - @discordjs/voice 0.19.0+ supports native AES-256-GCM
// If native crypto is available, no additional libraries are needed
// Otherwise, we need one of the optional encryption libraries
const crypto = require('node:crypto');
const hasNativeAES = crypto.getCiphers().includes('aes-256-gcm');

if (hasNativeAES) {
    console.log('Using native AES-256-GCM for encryption (no additional libraries needed)');
} else {
    console.log('Native AES-256-GCM not available, checking for alternative encryption libraries...');
    
    // Try to load optional encryption libraries (only needed if native AES-256-GCM unavailable)
    // Priority order (as per @discordjs/voice documentation):
    // 1. sodium-native (fastest, but requires native compilation)
    // 2. sodium (pure JS, works everywhere)
    // 3. tweetnacl (pure JS, lightweight fallback)
    // 4. libsodium-wrappers (pure JS, requires async initialization)
    let encryptionLoaded = false;

    // Try sodium-native first (fastest)
    try {
        require('sodium-native');
        console.log('Loaded sodium-native for encryption (native, fastest)');
        encryptionLoaded = true;
    } catch (e) {
        // Try sodium (pure JS, no async init needed)
        try {
            require('sodium');
            console.log('Loaded sodium (pure JS) for encryption - recommended for Windows');
            encryptionLoaded = true;
        } catch (e2) {
            // Try tweetnacl (pure JS, lightweight)
            try {
                require('tweetnacl');
                console.log('Loaded tweetnacl (pure JS) for encryption');
                encryptionLoaded = true;
            } catch (e3) {
                // Try libsodium-wrappers (pure JS, but needs async initialization)
                try {
                    require('libsodium-wrappers');
                    console.log('Loaded libsodium-wrappers (pure JS) for encryption');
                    encryptionLoaded = true;
                } catch (e4) {
                    console.error('WARNING: No encryption library found! Voice features may not work.');
                    console.error('Please run: npm install --ignore-scripts in services/voice_bridge');
                    console.error('Error loading sodium-native:', e.message);
                    console.error('Error loading sodium:', e2.message);
                    console.error('Error loading tweetnacl:', e3.message);
                    console.error('Error loading libsodium-wrappers:', e4.message);
                }
            }
        }
    }

    if (!encryptionLoaded) {
        console.error('CRITICAL: Encryption library not available. Voice features will not work.');
        console.error('Install one of: sodium, tweetnacl, libsodium-wrappers, or sodium-native');
    }
}

const {
    joinVoiceChannel,
    Constants,
    EndBehaviorType,
    VoiceConnectionStatus,
    entersState,
    createAudioPlayer,
    createAudioResource,
    StreamType,
    AudioPlayerStatus
} = require('@discordjs/voice');
// const { Client, GatewayIntentBits } = require('discord.js'); // Not used in External Mode
const WebSocket = require('ws');
const dgram = require('dgram');
const fs = require('fs');

// Configuration
const HTTP_PORT = 3000;
const UDP_PORT = 4000;
const GO_HOST = '127.0.0.1';

// We will use a WebSocket server to receive commands from Go
const wss = new WebSocket.Server({ port: 5000 });

console.log('Voice Bridge Service started (External Client Mode)');
console.log('Waiting for connections on WS port 5000');

// Map to hold active connections
const connections = new Map(); // guildId -> { adapter, connection, receiver }
const players = new Map(); // guildId -> AudioPlayer

// Helper to create a dummy client to satisfy @discordjs/voice adapter
function createAdapter(guildId) {
    return (methods) => {
        // Store the adapter methods to callback later
        connections.set(guildId, {
            adapter: methods,
            connection: null,
            receiver: null
        });

        return {
            sendPayload: (payload) => {
                console.log(`[Adapter] Sending payload for guild ${guildId} back to Go`);
                broadcastToGo({ op: 'FORWARD_PAYLOAD', data: payload }); // Use op/data structure
                return true;
            },
            destroy: () => {
                console.log(`[Adapter] Destroyed for guild ${guildId}`);
                connections.delete(guildId);
                if (players.has(guildId)) {
                    players.get(guildId).stop();
                    players.delete(guildId);
                }
            }
        };
    };
}

wss.on('connection', (ws) => {
    console.log('Go backend connected');

    ws.on('message', async (message) => {
        try {
            const data = JSON.parse(message);
            handleMessage(data, ws);
        } catch (e) {
            console.error('Failed to parse message:', e);
        }
    });
});

function broadcastToGo(msg) {
    wss.clients.forEach(client => {
        if (client.readyState === WebSocket.OPEN) {
            client.send(JSON.stringify(msg));
        }
    });
}

function handleMessage(msg, ws) {
    console.log('Received command:', msg.op);
    if (msg.data) {
        console.log('Command data:', JSON.stringify(msg.data, null, 2));
    }

    switch (msg.op) {
        case 'JOIN':
            console.log('Handling JOIN command...');
            handleJoin(msg.data);
            break;
        case 'VOICE_SERVER_UPDATE':
            console.log('Handling VOICE_SERVER_UPDATE...');
            handleVoiceServerUpdate(msg.data);
            break;
        case 'VOICE_STATE_UPDATE':
            console.log('Handling VOICE_STATE_UPDATE...');
            handleVoiceStateUpdate(msg.data);
            break;
        case 'PLAY':
            console.log('Handling PLAY command...');
            handlePlay(msg.data);
            break;
        default:
            console.warn('Unknown command:', msg.op);
    }
}

async function handleJoin(data) {
    const { guildId, channelId, selfDeaf, selfMute } = data;
    console.log(`Joining voice channel: ${channelId} in guild ${guildId}`);

    // Create the connection using @discordjs/voice
    try {
        const connection = joinVoiceChannel({
            channelId: channelId,
            guildId: guildId,
            adapterCreator: createAdapter(guildId), // Our custom adapter
            selfDeaf: false,
            selfMute: false, // We want to speak/listen
            group: 'default'
        });

        // Update our map
        const connData = connections.get(guildId);
        if (connData) {
            connData.connection = connection;
        }

        // Setup receivers once ready
        connection.on(VoiceConnectionStatus.Ready, () => {
            console.log('Voice connection ready!');
            setupReceiver(connection, guildId);
        });

        connection.on(VoiceConnectionStatus.Disconnected, () => {
            console.log('Voice connection disconnected');
        });

        connection.on(VoiceConnectionStatus.Signalling, () => {
            console.log('Voice connection signalling...');
        });

        connection.on(VoiceConnectionStatus.Connecting, () => {
            console.log('Voice connection connecting...');
        });

        connection.on("error", (error) => {
            console.error('Voice connection error:', error);
        });

        console.log('Voice connection initiated, waiting for ready state...');

    } catch (e) {
        console.error('Failed to join voice channel:', e);
        console.error('Error stack:', e.stack);
    }
}

function handlePlay(data) {
    const { guildId, type, path } = data;
    console.log(`Playing audio for guild ${guildId}: ${path} (${type})`);

    const connData = connections.get(guildId);
    if (!connData || !connData.connection) {
        console.warn(`No connection for guild ${guildId}, cannot play`);
        return;
    }

    // Check if connection is ready
    if (connData.connection.state.status !== VoiceConnectionStatus.Ready) {
        console.warn(`Connection not ready for guild ${guildId}, status: ${connData.connection.state.status}`);
        // Wait for ready state
        entersState(connData.connection, VoiceConnectionStatus.Ready, 5000)
            .then(() => {
                console.log(`Connection ready, retrying playback for guild ${guildId}`);
                handlePlay(data); // Retry
            })
            .catch(err => {
                console.error(`Failed to wait for ready state: ${err.message}`);
            });
        return;
    }

    // Check if file exists
    if (!fs.existsSync(path)) {
        console.error(`Audio file not found: ${path}`);
        return;
    }

    let player = players.get(guildId);
    if (!player) {
        player = createAudioPlayer();
        players.set(guildId, player);
        connData.connection.subscribe(player);

        player.on('error', error => {
            console.error(`Audio player error: ${error.message}`);
        });

        player.on(AudioPlayerStatus.Idle, () => {
            console.log('Audio finished, player idle');
        });

        player.on(AudioPlayerStatus.Playing, () => {
            console.log('Audio player is now playing');
        });

        player.on(AudioPlayerStatus.Paused, () => {
            console.log('Audio player is paused');
        });
    } else {
        // Stop current playback if playing
        if (player.state.status === AudioPlayerStatus.Playing || player.state.status === AudioPlayerStatus.Buffering) {
            console.log('Stopping current playback to start new audio');
            player.stop();
        }
    }

    try {
        console.log(`Creating audio resource from: ${path}`);
        const resource = createAudioResource(path, {
            inputType: StreamType.Arbitrary, // Let ffmpeg detect
            inlineVolume: true
        });

        console.log(`Playing resource on player for guild ${guildId}`);
        player.play(resource);
        console.log('Playback started successfully');

    } catch (e) {
        console.error('Failed to create audio resource:', e);
        console.error('Error stack:', e.stack);
    }
}

// These updates come from Go (who got them from Gateway)
function handleVoiceServerUpdate(data) {
    console.log('Received VOICE_SERVER_UPDATE');
    // Find the adapter and feed it
    // data is { token, endpoint, guild_id }
    const connData = connections.get(data.guild_id);
    if (connData && connData.adapter) {
        connData.adapter.onVoiceServerUpdate(data);
    } else {
        console.warn('No adapter found for guild', data.guild_id);
    }
}

function handleVoiceStateUpdate(data) {
    console.log('Received VOICE_STATE_UPDATE');
    // data is { session_id, channel_id, guild_id, ... }
    const connData = connections.get(data.guild_id);
    if (connData && connData.adapter) {
        connData.adapter.onVoiceStateUpdate(data);
    } else {
        console.warn('No adapter found for guild', data.guild_id);
    }
}

function setupReceiver(connection, guildId) {
    const receiver = connection.receiver;

    // Subscribe to speaking events
    receiver.speaking.on('start', (userId) => {
        console.log(`User ${userId} started speaking`);

        const pipeline = receiver.subscribe(userId, {
            end: {
                behavior: EndBehaviorType.AfterSilence,
                duration: 1000,
            },
        });

        const Prism = require('prism-media');
        const decoder = new Prism.opus.Decoder({ frameSize: 960, channels: 1, rate: 48000 });
        const sender = dgram.createSocket('udp4');

        // Handle decoder errors gracefully - opusscript can have decode errors on invalid packets
        decoder.on('error', (error) => {
            // Log but don't crash - invalid packets can happen
            console.warn(`Opus decode error for user ${userId}:`, error.message);
            // Don't propagate the error - just log it
        });

        // Handle pipeline errors
        pipeline.on('error', (error) => {
            console.warn(`Pipeline error for user ${userId}:`, error.message);
        });

        const pcmStream = pipeline.pipe(decoder);

        pcmStream.on('data', (chunk) => {
            try {
                // Header: [UserIDLen][UserID][PCMData]
                const userIdBuf = Buffer.from(userId);
                const header = Buffer.alloc(1);
                header.writeUInt8(userIdBuf.length, 0);

                const packet = Buffer.concat([header, userIdBuf, chunk]);

                sender.send(packet, UDP_PORT, GO_HOST, (err) => {
                    if (err) console.error('UDP send error:', err);
                });
            } catch (err) {
                console.warn(`Error processing PCM data for user ${userId}:`, err.message);
            }
        });

        pcmStream.on('error', (error) => {
            // Handle stream errors gracefully - don't crash
            console.warn(`PCM stream error for user ${userId}:`, error.message);
        });

        pcmStream.on('end', () => {
            // console.log(`Stream for ${userId} ended`);
            sender.close();
        });
    });
}
