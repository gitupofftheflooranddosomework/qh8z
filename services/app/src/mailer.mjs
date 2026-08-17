import nodemailer from 'nodemailer';
import { config } from './config.mjs';

let transport;
let lastVerify = { at: 0, ok: false };

function htmlEscape(value) {
  return String(value).replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]));
}

function getTransport() {
  if (config.mailMode !== 'smtp') return null;
  if (!transport) {
    transport = nodemailer.createTransport({
      host: config.smtpHost,
      port: config.smtpPort,
      secure: config.smtpSecure,
      requireTLS: config.smtpRequireTls && !config.smtpSecure,
      auth: config.smtpUser ? { user: config.smtpUser, pass: config.smtpPass } : undefined,
      connectionTimeout: 5000,
      greetingTimeout: 5000,
      socketTimeout: 10000,
    });
  }
  return transport;
}

export function mailerConfigured() {
  return config.mailMode === 'log' || Boolean(config.smtpHost && config.mailFrom);
}

export async function mailerHealthy() {
  if (config.mailMode === 'log') return true;
  const now = Date.now();
  if (now - lastVerify.at < 60_000) return lastVerify.ok;
  try {
    await getTransport().verify();
    lastVerify = { at: now, ok: true };
    return true;
  } catch {
    lastVerify = { at: now, ok: false };
    return false;
  }
}

async function send({ to, subject, text, html }) {
  if (config.mailMode === 'log') {
    console.log(JSON.stringify({ level: 'info', event: 'mail.log', to, subject, text }));
    return { logged: true };
  }
  if (!mailerConfigured()) throw new Error('Email delivery is not configured');
  return getTransport().sendMail({ from: config.mailFrom, to, subject, text, html });
}

export function sendVerificationEmail(user, token) {
  const url = `${config.appBaseUrl}/verify#${encodeURIComponent(token)}`;
  return send({
    to: user.email,
    subject: 'Verify your QH8Z email',
    text: `Welcome to QH8Z. Verify your email to create short links: ${url}\n\nThis link expires in 24 hours.`,
    html: `<h2>Welcome to QH8Z.</h2><p>Verify your email before creating short links.</p><p><a href="${htmlEscape(url)}">Verify email</a></p><p>This link expires in 24 hours.</p>`,
  });
}

export function sendPasswordResetEmail(user, token) {
  const url = `${config.appBaseUrl}/reset#${encodeURIComponent(token)}`;
  return send({
    to: user.email,
    subject: 'Reset your QH8Z password',
    text: `Reset your QH8Z password: ${url}\n\nThis link expires in 60 minutes. If you did not request it, ignore this email.`,
    html: `<h2>Reset your QH8Z password.</h2><p><a href="${htmlEscape(url)}">Choose a new password</a></p><p>This link expires in 60 minutes. If you did not request it, you can ignore this email.</p>`,
  });
}
