export function dependencyUnavailable(code, publicMessage, cause, event = 'dependency.unavailable') {
  console.warn(JSON.stringify({
    level: 'warn',
    event,
    dependencyCode: code,
    message: cause?.message || String(cause || 'unknown dependency failure'),
  }));
  const error = new Error(publicMessage, { cause });
  error.status = 503;
  error.code = code;
  return error;
}
