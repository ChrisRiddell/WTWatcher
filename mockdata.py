import json
import random
from datetime import datetime, timedelta, timezone

# Parameters
start_date = datetime(2026, 8, 17, tzinfo=timezone.utc)
days = 14
entries_per_day = 50

# Speedtest parameters
speedtests_min_per_day = 1
speedtests_max_per_day = 3

metrics_data = {}

total_latency_entries = 0
total_speedtest_entries = 0


def get_packet_loss() -> int:
    """
    Generate packet loss percentage.

    90% chance of 0% packet loss.
    10% chance of a minor spike between 1% and 20%.
    """
    return 0 if random.random() > 0.1 else random.randint(1, 20)


def get_speedtest() -> dict[str, float]:
    """
    Generate a realistic speed test result.

    Most results are close to the expected connection speed.
    Occasionally simulate a degraded connection.
    """
    download = random.uniform(80.0, 100.0)
    upload = random.uniform(18.0, 25.0)

    # Occasionally simulate a degraded connection.
    if random.random() < 0.15:
        download *= random.uniform(0.35, 0.75)
        upload *= random.uniform(0.40, 0.80)

    return {
        "download": round(download, 2),
        "upload": round(upload, 2),
    }


for day_offset in range(days):
    current_date = start_date + timedelta(days=day_offset)
    date_key = current_date.strftime("%Y-%m-%d")
    metrics_data[date_key] = {}

    # Generate latency timestamps.
    latency_timestamps = sorted(random.sample(range(86400), entries_per_day))

    # Generate 1-3 speed test timestamps per day.
    speedtest_count = random.randint(
        speedtests_min_per_day,
        speedtests_max_per_day,
    )

    speedtest_timestamps = random.sample(
        range(86400),
        speedtest_count,
    )

    # Combine timestamps so latency and speedtest data can
    # exist at the same timestamp without overwriting each other.
    all_timestamps = sorted(set(latency_timestamps + speedtest_timestamps))

    for sec in all_timestamps:
        timestamp = datetime.min.replace(tzinfo=timezone.utc) + timedelta(seconds=sec)

        time_key = timestamp.strftime("%H:%M:%SZ")
        entry = {}

        # Add latency data.
        if sec in latency_timestamps:
            entry["latency"] = {
                "Gateway": [
                    {
                        "average": round(
                            random.uniform(2.0, 8.0),
                            2,
                        ),
                        "packetLoss": get_packet_loss(),
                        "protocol": "IPv4",
                    }
                ],
                "Cloudflare DNS": [
                    {
                        "average": round(
                            random.uniform(11.0, 18.0),
                            2,
                        ),
                        "packetLoss": get_packet_loss(),
                        "protocol": "IPv6",
                    },
                    {
                        "average": round(
                            random.uniform(11.0, 17.0),
                            2,
                        ),
                        "packetLoss": get_packet_loss(),
                        "protocol": "IPv4",
                    },
                ],
                "Youtube": [
                    {
                        "average": round(
                            random.uniform(25.0, 40.0),
                            2,
                        ),
                        "packetLoss": get_packet_loss(),
                        "protocol": "IPv6",
                    },
                    {
                        "average": round(
                            random.uniform(27.0, 42.0),
                            2,
                        ),
                        "packetLoss": get_packet_loss(),
                        "protocol": "IPv4",
                    },
                ],
            }

            total_latency_entries += 1

        # Add speedtest data.
        if sec in speedtest_timestamps:
            entry["speedtest"] = [get_speedtest()]
            total_speedtest_entries += 1

        metrics_data[date_key][time_key] = entry


# Save to metrics.json
output_file = "./public/metrics.json"

with open(output_file, "w", encoding="utf-8") as file:
    json.dump(metrics_data, file, indent=2)


print(
    f"Successfully generated {days} days of data with "
    f"{total_latency_entries} latency entries and "
    f"{total_speedtest_entries} speed tests in {output_file}"
)
