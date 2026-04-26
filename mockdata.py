import json
import random
from datetime import datetime, time, timedelta, timezone

# Parameters (Added tzinfo=timezone.utc)
start_date = datetime(2026, 8, 17, tzinfo=timezone.utc)
days = 14
entries_per_day = 50

metrics_data = {}


def get_packet_loss():
    # 90% chance of 0% packet loss, 10% chance of a minor spike (1% to 20%)
    return 0 if random.random() > 0.1 else random.randint(1, 20)


for day_offset in range(days):
    current_date = start_date + timedelta(days=day_offset)
    date_key = current_date.strftime("%Y-%m-%d")
    metrics_data[date_key] = {}

    # Sample random timestamps throughout the day
    timestamps = sorted(random.sample(range(86400), entries_per_day))

    for sec in timestamps:
        time_key = (
            time(0, 0)
            if sec == 0
            else (
                datetime.min.replace(tzinfo=timezone.utc) + timedelta(seconds=sec)
            ).time()
        ).strftime("%H:%M:%SZ")

        metrics_data[date_key][time_key] = {
            "latency": {
                "Cloudflare DNS": [
                    {
                        "average": round(random.uniform(11.0, 18.0), 2),
                        "packetLoss": get_packet_loss(),
                        "protocol": "IPv6",
                    },
                    {
                        "average": round(random.uniform(11.0, 17.0), 2),
                        "packetLoss": get_packet_loss(),
                        "protocol": "IPv4",
                    },
                ],
                "Gateway": [
                    {
                        "average": round(random.uniform(2.0, 8.0), 2),
                        "packetLoss": get_packet_loss(),
                        "protocol": "IPv4",
                    }
                ],
                "Youtube": [
                    {
                        "average": round(random.uniform(25.0, 40.0), 2),
                        "packetLoss": get_packet_loss(),
                        "protocol": "IPv6",
                    },
                    {
                        "average": round(random.uniform(27.0, 42.0), 2),
                        "packetLoss": get_packet_loss(),
                        "protocol": "IPv4",
                    },
                ],
            }
        }

# Save to metrics.json
with open("./public/metrics.json", "w") as f:
    json.dump(metrics_data, f, indent=2)

print("Successfully generated 14 days of data (~700 entries) in metrics.json")
