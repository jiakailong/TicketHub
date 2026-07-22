import client from './client.js'

export const userApi = {
  register: (payload) => client.post('/users/register', payload),
  createRegisterCaptcha: (payload) => client.post('/users/register/captcha', payload),
  login: (payload) => client.post('/users/login', payload),
  detail: () => client.get('/users/detail'),
  ticketUsers: () => client.get('/users/ticket-users'),
  addTicketUser: (payload) => client.post('/users/ticket-users', payload),
}

export const programApi = {
  search: (params = {}) => client.get('/programs/search', { params }),
  detail: (programId, ticketCategoryId) => client.get('/programs/detail', {
    params: { program_id: programId, ticket_category_id: ticketCategoryId || undefined },
  }),
}

export const orderApi = {
  create: (payload, requestId) => client.post('/orders', payload, { headers: { 'Idempotency-Key': requestId } }),
  list: (params = {}) => client.get('/orders', { params: { limit: 20, ...params } }),
  detail: (orderNumber, silent = false) => client.get('/orders', { params: { order_number: orderNumber }, silent }),
  cancel: (orderNumber) => client.post('/orders/cancel', { order_number: orderNumber }),
}

export const paymentApi = {
  create: (payload) => client.post('/payments', payload),
  check: (orderNumber, channel = 'mock') => client.get('/payments/check', { params: { order_number: orderNumber, channel } }),
  callback: (payload) => client.post('/payments/callback', payload),
}
