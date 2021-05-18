import json
import math

EARTH_RADIUS = 6.3781 * 1e6


def travel_time(p1, p2, speed, sensing_time):
    dx = (p2[1] - p1[1]) * math.cos(
        0.5 * (p1[0] + p2[0]) * math.pi / 180) * math.pi / 180 * EARTH_RADIUS
    dy = (p2[0] - p1[0]) * math.pi / 180 * EARTH_RADIUS
    dist = math.sqrt(dx**2 + dy**2)
    flight_time = dist / speed
    return math.ceil(flight_time + sensing_time)


def merge_ims(ims):
    im = {}
    for x in ims:
        for t in x:
            im[t] = x[t].copy()
    return im


def get_apps(im):
    apps = []
    for k in im.keys():
        apps += [k[2]]
    return set(apps)


def get_schedule_stats(im, schedule):
    elapsed_times = []
    current_interests = {a: 0.0 for a in get_apps(im)}
    if schedule:
        for route in schedule:
            elapsed_times += [route['total_time']]
            for task in route['path']:
                key = (task['location']['latitude'], task['location']['longitude'], task['app_id'], task['request_time'])
                current_interests[key[2]] += float(im[key]['interest'])

    return current_interests, elapsed_times


def reweight(im, w):
    # check that there is a weight for each app
    assert (len(w) == len(get_apps(im)))

    imw = {}
    for t in im.keys():
        imw[t] = im[t].copy()
        imw[t]['interest'] = w[t[2]] * im[t]['interest']
    return imw


def save_json(data, path):
    with open(path, "w") as x:
        json.dump(data, x, indent=4)
