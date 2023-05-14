import matplotlib.pyplot as plt


poc_dir = "PATH_TO_POC_DATA_SET"
poc_cilium_dir = "PATH_TO_CILIM_DATA_SET"
# Limit of divergent requests.
upper_lim = 2000


def get_values(filename, upperBoundAnomaly, targetLoc):
    with open(filename) as f:
        x_values = []
        y_values = []
        toggle = 0
        for line in f:
            if not toggle:
                if "ALL DATA:" in line:
                    toggle = 1
                    f.readline()
                    continue
            else:
                values = [s.strip('[]\n') for s in line.split(',')]
                print(values)
                if len(values) == 3:
                    if values[1] == targetLoc:
                        latency = int(values[2])
                        num_req = int(values[0])
                        if latency > upperBoundAnomaly:
                            continue
                        y_values.append(latency)
                        x_values.append(num_req)
        return (x_values, y_values)


def get_average_trim(listValues, trim):
    toAverage = []
    for values in listValues:
        print("values...")
        print(len(values))
        toAverage.append(values[:trim])
    print("sum")
    return [int(sum(values) / len(values)) for values in zip(*toAverage)]


def plot_all(file_dir, file_name, target_loc, upper_anomaly, num_points,
             num_files, title, y_label, x_label):
    path = file_dir+file_name
    for id in range(num_files):
        x, y = get_values(path+str(id)+".txt", upper_anomaly, target_loc)
        plt.plot(x[:num_points], y[:num_points])

    plt.title(title)
    plt.ylabel(y_label)
    plt.xlabel(x_label)
    plt.show()


def plot_avg(file_dir, file_name, target_loc, upper_anomaly, num_points,
             num_files, title, y_label, x_label):
    path = file_dir+file_name
    avg = []
    for id in range(num_files):
        x, y = get_values(path+str(id)+".txt", upper_anomaly, target_loc)
        avg.append(y)
    avg_latency = get_average_trim(avg, num_points)
    plt.plot(list(range(num_points)), avg_latency)

    plt.title(title+" [avg of ("+str(num_files)+") runs]")
    plt.ylabel(y_label)
    plt.xlabel(x_label)
    plt.show()

# POC - 500hz
#plot_avg(poc_dir, "poc_20pods_cyc2_hz500_", "east",1500, 400, 5, "PoC - requests sent at freq 500(Hz)", "Latency (ms)", "Request num")
#plot_all(poc_dir, "poc_20pods_cyc2_hz500_", "east", 1500, 400, 3, "requests sent at freq 500(Hz)", "Latency (ms)", "Request num")

# POC - 1000hz
# 250-400
#plot_avg(poc_dir, "poc_20pods_cyc2_hz1000_", "east", 1500, 400, 5, "PoC - requests sent at freq 1000(Hz)", "Latency (ms)", "Request num")
#plot_all(poc_dir, "poc_20pods_cyc2_hz1000_", "east", 1500, 800, 5, "requests sent at freq 1000(Hz)", "Latency (ms)", "Request num")

# CILIUM - 500hz
#plot_avg(poc_cilium_dir, "500hz_cilium_", "west", 1500, 400, 5, "Cilium with ingress proxy - requests sent at freq 500(Hz)", "Latency (ms)", "Request num")
#plot_all(poc_cilium_dir, "500hz_cilium_", "west",1500, 800, 3, "Cilium with ingress proxy - requests sent at freq 500(Hz)", "Latency (ms)", "Request num")

# CILIUM - 1000hz
#plot_avg(poc_cilium_dir, "1000hz_cilium_", "west", 1500, 800, 5, "Cilium with ingress proxy - requests sent at freq 1000(Hz)", "Latency (ms)", "Request num")
#plot_all(poc_cilium_dir, "1000hz_cilium_", "west", 1500, 800, 3, "Cilium with ingress proxy - requests sent at freq 1000(Hz)", "Latency (ms)", "Request num")
