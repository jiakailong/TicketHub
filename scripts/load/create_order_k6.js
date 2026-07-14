import http from "k6/http";
import { check, sleep } from "k6";

export const options = {
  scenarios: {
    flash_sale: {
      executor: "ramping-vus",
      stages: [
        { duration: "30s", target: 50 },
        { duration: "1m", target: 200 },
        { duration: "30s", target: 0 }
      ]
    }
  },
  thresholds: {
    http_req_failed: ["rate<0.02"],
    http_req_duration: ["p(95)<800"]
  }
};

const baseURL = __ENV.BASE_URL || "http://127.0.0.1:8080";
const token = __ENV.TOKEN || "";

export default function () {
  const payload = JSON.stringify({
    program_id: Number(__ENV.PROGRAM_ID || 10001),
    ticket_category_id: Number(__ENV.TICKET_CATEGORY_ID || 1),
    seat_ids: [Number(__ENV.SEAT_ID || 100)],
    ticket_user_ids: [Number(__ENV.TICKET_USER_ID || 50001)]
  });

  const params = {
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`
    }
  };

  const res = http.post(`${baseURL}/api/orders`, payload, params);
  check(res, {
    "order accepted": (r) => r.status === 200 || r.status === 409 || r.status === 422
  });
  sleep(1);
}
